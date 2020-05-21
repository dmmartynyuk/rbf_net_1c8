package main

import (
	"1c8_zak/models"
	"1c8_zak/rbfnet"
	"log"
	"math"
	"sort"
	"strconv"
	"time"
)

//calcnet прогнозирунт продажи для склада kStore и товара kGoods. Если kGoods="", то прогноз для всего магазина
func calcnet(kStore string, kGoods string) (retNext int, retDemand float64) {
	ver := 2
	var PROGPER, KC, INF, MAXSALES, prgper, lkc int
	var stores *models.Stores
	var guess = make([]float64, 3)
	var predict = make([]float64, 3)
	var now float64 = float64(int64(time.Now().Unix() / 86400)) //сегодня
	var goods []models.MatrixGoods
	var islog = true

	retNext = 0
	retDemand = 0.0

	if len(kGoods) > 1 {
		islog = false
	}
	Mc.Set(time.Now().UTC().UnixNano())
	if islog {
		err := models.DbLog("beg. начало составления прогноза", "calculate", Mc.Val())
		if err != nil {
			log.Println("Ошибка лога " + strconv.FormatInt(Mc.Val(), 10) + " " + err.Error())
			Mc.Set(0)
			return
		}
	}
	defer func() {
		if r := recover(); r != nil {
			models.DbLog("err. конец составления прогноза по recover "+r.(error).Error(), "calculate", time.Now().UTC().UnixNano())
			log.Printf("err. конец составления прогноза по recover")
		}
		if islog {
			models.DbLog("end. конец составления прогноза", "calculate", time.Now().UTC().UnixNano())
		}
	}()
	defer Mc.Set(0)

	conf, err := models.GetConfig()
	PROGPER = conf.ValInt("minday2sale", 7)
	KC = conf.ValInt("kfnunvisible", 3)     //коэф сети
	INF = conf.ValInt("inf", 30)            //удаленнность от последнего значения, см использование
	MAXSALES = conf.ValInt("maxsales", 720) //глубина просмотра продаж для составления статистики
	//contracts, err = models.GetContracts()

	stores, err = models.GetMagNames(0, kStore)

	if err != nil {
		models.DbLog("calc. Ошибка чтения склада "+err.Error(), "calculate", time.Now().UTC().UnixNano())
		return
		//log.Fatalf("Ошибка чтения склада %s", err)
	}
	//по всем магазинам из stores
	for uidstore, namest := range *stores {
		var sales = new(models.Sales)
		goods, err = models.GetAllGoodsFromMatrix(uidstore, kGoods)

		if err != nil {
			models.DbLog("calc. Ошибка чтения  матрицы магазина "+namest+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
			return
			//log.Fatalf("Ошибка чтения матрицы магазина %s", namest)
		}
		for _, merch := range goods {
			if Mc.Val() < 0 {
				//надо завершиться
				Mc.Set(0)
				models.DbLog("stop. завершено по сигналу", "calculate", time.Now().UTC().UnixNano())
				log.Printf("stop. завершено по сигналу")
				return
			}
			lkc = KC

			//если прогноз делался не так давно, то возвращаем результаты последнего прогноза
			lp, err := time.Parse("2006-01-02", merch.PredPeriod)
			if lp.AddDate(0, 0, (int)(merch.PredDays/2)).Unix() >= time.Now().Unix() {
				retNext = merch.PredDays
				retDemand = merch.PredDemand
				continue
			}

			predict[0] = 0.0
			predict[1] = 0.0
			sales, err = models.GetSales(uidstore, merch.KeyGoods, time.Now().AddDate(0, 0, -MAXSALES).Format("2006-01-02"))
			cnt := len(sales.Cnt)
			if err != nil {
				models.DbLog("calc. Ошибка чтения продаж магазина "+namest+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
				return
				//log.Fatalf("Ошибка чтения продаж магазина %s %v", namest, err)
			}
			if cnt == 0 {
				continue
			}
			gt, err := models.GetGoods(merch.KeyGoods, "")
			if err != nil {
				models.DbLog("calc. Ошибка чтения товара "+merch.KeyGoods+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
				return
				//log.Fatalf("Ошибка чтения товара %s %v", merch, err)
			}
			//log.Println(namest)
			firstdeal := sales.Udate[0]
			lastdeal := sales.Udate[cnt-1]
			//sales.Prevdays частота продаж в днях, от предидущей покупки
			meanpd, sigmapd, stat := rbfnet.GetSigma(sales.Prevdays)
			meancnt, sigmacnt, statcnt := rbfnet.GetSigma(sales.Cnt)
			//если мало входных данных, то просто вычисляем по статистике
			if cnt < KC*2 {
				//если движение по складу было недавно, то закажем по последнему движению, иначе 0
				if now-lastdeal < 20 {
					//for i, z := cnt-1, 0; i >= 0 && z < 3; i-- {
					//	predict[z] = sales.Cnt[i]
					//	z++
					//}
					d := 0.0
					if meanpd != 0 {
						d = meancnt / meanpd
					}
					retDemand = d
					retNext = int(sigmapd)
					models.SavePredict(uidstore, merch.KeyGoods, meancnt, now, int(sigmapd), d)
					//log.Printf("merch=%s %s, lastmov=%v balance=%v period=%v, prognoz=%v cnt=%v\n", gt.Art, gt.Name, time.Unix(int64(sales.LastPeriod*86400), 0).Format("2006-01-02"), sales.LastBalance, time.Unix(int64((now+float64(prgper))*86400), 0).Format("2006-01-02"), predict, int(predict[0]))
					continue
				} else {
					//давно дело было, ничего прогнозировать по разовым сделкам не будем
					continue
				}
			}
			//log.Printf("merch=%s %s\n", gt.Art, gt.Name)
			//1. расчет количеста дней, в течении которых с вероятностью более 99% (3*сигма) купят хотябы один товар
			//помним, что в пределах двух сигм лежит 95% , в пределах 3сигм более 99.7%

			//входные данные сети
			inputs := make([]float64, cnt+1)
			//прогноз по частоте покупок и по количеству
			for k := range sales.Prevdays {
				//sales.Udate это номер дня Unix-число. Приводим к виду 1,2,3,4...
				inputs[k] = float64(k + 1)
			}
			guess[0] = inputs[cnt-1] + 1.0
			guess[1] = inputs[cnt-1] + 2.0
			guess[2] = inputs[cnt-1] + 3.0
			//v3-> привяжем в бесконечности к середине meanpd, чтобы функция не уполхала в бесконечность
			if cnt < 30 {
				inputs[cnt] = guess[2] + float64(cnt)
			} else {
				inputs[cnt] = guess[2] + float64(INF)
			}
			// <-v3
			//если входных данных много, то часть берем для обучения, а три последних для проверки
			//Стаптистика по периодичности продаж
			centers := rbfnet.MakeCenters(inputs[0:cnt], lkc)
			centers = append(centers, inputs[cnt])
			r := rbfnet.NewRBFNetwork(len(inputs), len(centers), float64(KC), centers)
			outputs := make([]float64, cnt+1)
			copy(outputs, sales.Prevdays)
			outputs[cnt] = meanpd
			if r.Train(inputs, outputs, 1000) < 0 {
				predict[0] = meanpd
				predict[1] = meanpd
			}
			//предсказания для guess
			copy(predict, r.Predict(guess))
			//если predict[0] больше мин макс входных значений на 30% переучиваем сеть уменьшая количество скрытых нейронов

			derivless := stat["deriv"] != 0 && stat["deriv"] < math.Abs(predict[0]-predict[1]) //наклон функции
			for i := 1; i < len(inputs) && (derivless || predict[0] > stat["max"]*1.3 || predict[0] < stat["min"]*0.7); i++ {
				//уменьшаем количество скрытых нейронов
				hidd := int(len(inputs)/KC) - i
				if hidd < 2 {
					break
				}
				//lkc = len(inputs) / hidd
				centers = rbfnet.MakeCenters2(inputs[0:cnt], hidd)
				centers = append(centers, inputs[cnt])
				r = rbfnet.NewRBFNetwork(len(inputs), len(centers), sigmapd, centers)
				r.Train(inputs, outputs, 1000)
				//log.Printf("ошибка сети %v\n", o)
				copy(predict, r.Predict(guess))
				derivless = stat["deriv"] < math.Abs(predict[0]-predict[1]) || predict[0] < 0
			}
			//следующая покупка будет через
			nextdeal := int(predict[0] + 0.5)
			if nextdeal <= 0 {
				nextdeal = 1
			}
			if derivless {
				nextdeal = int(meancnt)
			}
			//теперь прогноз количества следующей покупки
			p1 := 1.0
			//если разброс от центра маленький, то ничего считать не будем, берем центр
			if sigmacnt < 3 {
				p1 = meancnt
			} else {
				centers = rbfnet.MakeCenters(inputs[0:cnt], lkc)
				centers = append(centers, inputs[cnt])
				copy(outputs, sales.Cnt)
				outputs[cnt] = meancnt
				r = rbfnet.NewRBFNetwork(len(inputs), len(centers), sigmacnt, centers)
				r.Train(inputs, outputs, 1000)

				//log.Printf("ошика сети %v\n", o)
				copy(predict, r.Predict(guess))

				derivless = statcnt["deriv"] != 0 && statcnt["deriv"] < math.Abs(predict[0]-predict[1])
				for i := 1; i < len(inputs) && (derivless || predict[0] > statcnt["max"]*1.3 || predict[0] < statcnt["min"]*0.7); i++ {
					//log.Printf("merch=%v, prognoz=%v  в следующие %v дней купят %v штук\n", merch, predict, nextdeal, int(predict[0]+0.5))
					//уменьшаем количество скрытых нейронов
					hidd := int(len(inputs)/KC) - i
					if hidd < 2 {
						break
					}
					centers = rbfnet.MakeCenters2(inputs[0:cnt], hidd)
					centers = append(centers, inputs[cnt])
					r = rbfnet.NewRBFNetwork(len(inputs), len(centers), sigmacnt, centers)
					outputs := make([]float64, cnt+1)
					copy(outputs, sales.Cnt)
					outputs[cnt] = meancnt
					if r.Train(inputs, outputs, 1000) < 0 {
						predict[0] = meancnt
						break
					}
					//log.Printf("ошибка сети %v\n", o)
					copy(predict, r.Predict(guess))
					derivless = statcnt["deriv"] < math.Abs(predict[0]-predict[1])

				}
				p1 = predict[0]
				if predict[0] < 0 {
					p1 = statcnt["min"]
				}
			}
			log.Printf("store=%v merch=%s %s, prognoz=%v  в следующие %v дней купят %v штук\n", namest, gt.Art, gt.Name, predict, nextdeal, p1)

			//nextdeal := lastdeal + meanpd + 3*sigmapd
			//V1 demand
			//средняя потребность на день в периоде покупок
			demand := make([]float64, cnt+1)
			mn := 0.0
			for k, v := range sales.Prevdays {
				//возврат как правило на след. день. Это сильно функцию уводит вниз, для минусовых значений ставим около 0
				if v != 0 {
					demand[k] = sales.Cnt[k] / v
				} else {
					demand[k] = 0.000000001
				}
				if sales.Cnt[k] < 0 {
					demand[k] = 0.000000001
				}
				mn = mn + demand[k]
			}
			demand[cnt] = mn / float64(cnt-1)
			prgper = PROGPER
			//V1
			if ver == 1 {
				for k, v := range sales.Udate {
					//sales.Udate это номер дня Unix-число. Приводим к виду 1,2,3,4...
					inputs[k] = v - firstdeal + 1
				}
				//дни отсутствия товара на складе, от последней сделки до сегодняшнего дня не учитываем.
				//zperiod := lastdeal
				if sales.LastBalance == 0 {
					//firstdeal в inputs = 1
					//	zperiod = now - (lastdeal - firstdeal) //zerro point  g[0]+z=now
					guess[0] = lastdeal - firstdeal + 1 + float64(prgper)
					guess[1] = lastdeal - firstdeal + 1 + float64(prgper*2)
					guess[2] = lastdeal - firstdeal + 1 + float64(prgper*3)
				} else {
					//	zperiod = firstdeal //zerro point g[0]+z=now
					guess[0] = now - firstdeal + 1 + float64(prgper)
					guess[1] = now - firstdeal + float64(prgper*2)
					guess[2] = now - firstdeal + float64(prgper*3)
				}
				//end V1
			} else {
				//V2 inputs всегда 1,2,3,4,5....
				//т.к demand уже содержит в себе потребность на день, то входной ряд можно представить просто
				if sales.LastBalance > 0 {
					prgper = int(now-lastdeal) + PROGPER + 1
				}
				//end V2
			}
			//если sigmapd более 30 этот товар лцчше использовать как заказную позицию
			pd := int(meanpd + 2*sigmapd)
			if pd < 6 {
				pd = int(meanpd + 3*sigmapd)
			} else if pd > 1000 {
				//еслт очень мало данных
				pd = nextdeal
			}

			centers = rbfnet.MakeCenters(inputs[0:cnt], lkc)
			centers = append(centers, inputs[cnt])
			r = rbfnet.NewRBFNetwork(len(inputs), len(centers), float64(lkc), centers)
			if r.Train(inputs, demand, 1000) >= 0 {
				//предсказания для guess
				copy(predict, r.Predict(guess))
			} else {
				predict[0], _, _ = rbfnet.GetSigma(demand[0:cnt])
			}

			//predict[0] - это потребность на день, тогда потребность на следующие progper дней равна int(predict[0]*float64(progper)+0.5) , округляем до 1
			//настройки сети в json
			strrbf := r.DumpRBF()
			if predict[0] < 0 {
				predict[0] = p1 / float64(nextdeal)
			}
			retNext = pd
			retDemand = predict[0]
			//сохранили настройки сети
			models.SaveRbfNet(uidstore, merch.KeyGoods, string(strrbf), sales.LastPeriod, float64(pd))
			models.SavePredict(uidstore, merch.KeyGoods, p1, now, pd, predict[0])
			//log.Printf("merch=%s %s, lastmov=%v balance=%v period=%v, prognoz=%v cnt=%v\n", gt.Art, gt.Name, time.Unix(int64(sales.LastPeriod*86400), 0).Format("2006-01-02"), sales.LastBalance, time.Unix(int64((guess[0]+zperiod)*86400), 0).Format("2006-01-02"), predict, int(predict[0]*float64(progper)+0.7))
			//if now-lastdeal > float64(pd) && sales.LastBalance > 0 {
			//	log.Printf("должен быть продан pd=%v, а уже прошло %v", pd, now-lastdeal)
			//}
			//log.Printf("pd=%v", pd)
		}
		//break
	}
	return
}

//makeOrders делает таблицу заказов из predict
func apiMakeOrders() {
	var stores *models.Stores
	var err error
	var now = time.Now() //сегодня
	var provider string

	Fop.Set(time.Now().UTC().UnixNano())
	err = models.DbLog("beg makeOrders. Начало составления заказов", "makeOrders", Fop.Val())
	if err != nil {
		log.Println("Ошибка лога " + strconv.FormatInt(Fop.Val(), 10) + " " + err.Error())
		Fop.Set(0)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			models.DbLog("err. конец составления заказов по recover "+r.(error).Error(), "makeOrders", time.Now().UTC().UnixNano())
			log.Printf("err. конец составления заказов по recover")
		}
		models.DbLog("end. конец составления заказов", "makeOrders", time.Now().UTC().UnixNano())
	}()
	defer Fop.Set(0)

	conf, err := models.GetConfig()
	MINDAYSORD := conf.ValInt("minDaysOrd", 7)

	stores, err = models.GetMagNames(0, "")

	if err != nil {
		models.DbLog("makeOrders. Ошибка чтения склада "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
		return
	}
	numzak := 0
	//по всем магазинам читаем матрицу и делаем заказ
	for uidstore, namest := range *stores {
		isneedord := false
		delivdays := 1
		//на основе данных по доставке делаем заказ
		var contr []models.Contract
		contr, err = models.GetContracts(uidstore)
		if err != nil {
			//несмогли прочитать контракты, делаем заказ на завтра
			isneedord = true
		}
		if len(contr) > 0 {
			//смотрим частоту заказов на склад
			delivdays = 9999
			for _, v := range contr {
				if inChedule(v.Chedord) {
					if delivdays > v.Delivdays {
						delivdays = v.Delivdays
					}
					isneedord = true
					provider = v.Provider
				}
			}
		}
		if !isneedord {
			//если заказ не надо делать, то следующий склад
			continue
		}
		numzak++
		var ordersnum string
		if numzak < 10 {
			ordersnum = strconv.Itoa(now.YearDay()) + "0" + strconv.Itoa(numzak)
		} else {
			ordersnum = strconv.Itoa(now.YearDay()) + strconv.Itoa(numzak)
		}
		goods, err := models.GetAllGoodsFromMatrix(uidstore, "")
		if err != nil {
			models.DbLog("makeOrders. Ошибка чтения  матрицы магазина "+namest+" "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
			return
		}
		datedelivdays := now.AddDate(0, 0, delivdays).Format("2006-01-02")
		if delivdays < MINDAYSORD {
			delivdays = MINDAYSORD
		}
		for _, merch := range goods {
			if Fop.Val() < 0 {
				//надо завершиться
				Fop.Set(0)
				models.DbLog("stop. завершено по сигналу", "makeOrders", time.Now().UTC().UnixNano())
				log.Printf("stop. завершено по сигналу")
				return
			}
			/*
				lp, err := models.GetLastPredict(uidstore, merch.KeyGoods)
				if err != nil {
					models.DbLog("makeOrders. Ошибка чтения таблицы предсказаний "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
					return
				}*/
			//если товар заказан уже то не заказываем

			//lper дата последней продажи товара
			lper, err := time.Parse("2006-01-02T15:04:05", merch.PredPeriod)
			if err != nil {
				//нет даты прогноза или формат даты другой
				lper, err = time.Parse("2006-01-02", merch.PredPeriod)
				if err != nil {
					lper = time.Now()
				}
			}
			//следующий раз надо заказывать не ранее
			next := lper.AddDate(0, 0, merch.PredDays)
			//надо ли пересчистать статистику?
			if now.Unix() > next.Unix() {
				merch.PredDays, merch.PredDemand = calcnet(uidstore, merch.KeyGoods)
			}
			//надо заказать для склада
			cntzak := float64(delivdays) * merch.PredDemand
			//смотрим текущий остаток и минимальный остаток склада
			cntzak = cntzak - (merch.Balance - (merch.MinBalance + merch.Vitrina))
			if cntzak+merch.Balance > merch.MaxBalance {
				cntzak = merch.MaxBalance - merch.Balance
			}
			//надо заказывать кратно step
			if merch.Step > 1 {
				cntzak = float64(int(merch.Step) * int(cntzak/merch.Step+0.9999))
			}

			if cntzak > 0.0 && (int)(cntzak+0.5) > 0 {
				models.SaveOper(ordersnum, provider, uidstore, merch.KeyGoods, now.Format("2006-01-02"), cntzak, next.Format("2006-01-02"), datedelivdays)
			}
		}
	}
	models.DbLog("end makeOrders. Конец составления заказов", "makeOrders", time.Now().UTC().UnixNano())
}

//apiRecalcABC расчет АВС классификации товара для склада uidStore и товара uidGoods в течении периода period
func apiRecalcABC(uidStore string, period1 string, period2 string) error {
	lp, sum := models.GetProfit(uidStore, period1, period2)
	//отсортируем map по значению
	type kv struct {
		k string
		v float64
	}
	kvs := make([]kv, 0, len(lp))
	for k, v := range lp {
		kvs = append(kvs, kv{k, v})
	}
	//сортруем по убыванию значения прибыли v
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].v > kvs[j].v
	})
	var m map[string]interface{}
	m = map[string]interface{}{}
	w := make(map[string]string)
	w["uidStore"] = uidStore
	//80 percent of profit
	sumA := sum * 0.7
	sumB := sum * 0.9
	var s float64 = 0.0
	for _, v := range kvs {
		w["uidGoods"] = v.k
		s = s + v.v
		if s < sumA {
			//a tip
			m["abc"] = "A"
		} else {
			if s < sumB {
				//b tip
				m["abc"] = "B"
			} else {
				//c tip
				m["abc"] = "C"
			}
		}
		err := models.UpdateMatrix(m, w)
		if err != nil {
			//log.Println(err)
			return err
		}
	}
	return nil
}

//needPredict возвращает истину, если пора делать расчет по товару для склада
func needPredict(uidStore string, uidGoods string) bool {

	nr, err := models.GetLastPredict(uidStore, uidGoods)
	if err != nil {
		return true
	}
	//если делался почти
	lp, err := time.Parse("2006-01-02", nr.Period)
	if err != nil {
		return true
	}
	if lp.AddDate(0, 0, (int)(nr.Days/2)).Unix() >= time.Now().Unix() {
		return true
	}
	return false
}
