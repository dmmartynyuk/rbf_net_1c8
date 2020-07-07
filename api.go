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
	//var stores []models.Store
	var guess = make([]float64, 3)
	var predict = make([]float64, 3)
	var now float64 = float64(int64(time.Now().Unix() / 86400)) //сегодня
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
	KC = conf.ValInt("kfnunvisible", 4)     //коэф сети, для редких продаж лучше болье
	INF = conf.ValInt("inf", 30)            //удаленнность от последнего значения, см использование
	MAXSALES = conf.ValInt("maxsales", 720) //глубина просмотра продаж для составления статистики
	//contracts, err = models.GetContracts()

	stores, err := models.GetMagNames(0, kStore)

	if err != nil {
		models.DbLog("calc. Ошибка чтения склада "+err.Error(), "calculate", time.Now().UTC().UnixNano())
		return
		//log.Fatalf("Ошибка чтения склада %s", err)
	}
	//по всем магазинам из stores
	for _, store := range stores {
		uidstore := store.KeyStore
		var sales = new(models.Sales)
		var goods []models.MatrixGoods
		tipSales := "S"
		if store.Tip < 0 {
			continue
		}
		if store.Tip > 0 {
			goods, err = models.GetAllGoodsFromMatrix(uidstore, kGoods)
		} else {
			goods, err = models.GetOptMatrix(uidstore, kGoods, MAXSALES)
			//для оптового склада и продажи, и движения
			tipSales = "SM"
		}
		if err != nil {
			models.DbLog("calc. Ошибка чтения  матрицы магазина "+store.Name+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
			return
			//log.Fatalf("Ошибка чтения матрицы магазина %s", store.Name)
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

			//надо ли пересчистать статистику?
			/*
				recalc := false
				if merch.PredDays == 0 || (merch.PredDays > 0 && merch.PredDemand > (merch.PredCnt/float64(merch.PredDays))*20) {
					//ошибка сети, пересчитаем
					recalc = true
					lkc++
				}
				//если прогноз делался не так давно, то возвращаем результаты последнего прогноза
				lp, err := time.Parse("2006-01-02", merch.PredPeriod)
				if !recalc && lp.AddDate(0, 0, (int)(merch.PredDays/2)).Unix() >= time.Now().Unix() {
					retNext = merch.PredDays
					retDemand = merch.PredDemand
					continue
				}
			*/
			predict[0] = 0.0
			predict[1] = 0.0
			sales, err = models.GetSales(uidstore, merch.KeyGoods, time.Now().AddDate(0, 0, -MAXSALES).Format("2006-01-02"), time.Now().Format("2006-01-02"), tipSales)
			cnt := len(sales.Cnt)
			if err != nil {
				models.DbLog("calc. Ошибка чтения продаж магазина "+store.Name+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
				return
				//log.Fatalf("Ошибка чтения продаж магазина %s %v", store.Name, err)
			}
			if cnt == 0 {
				continue
			}
			gt, err := models.GetGood(merch.KeyGoods)
			if err != nil {
				models.DbLog("calc. Ошибка чтения товара "+merch.KeyGoods+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
				return
				//log.Fatalf("Ошибка чтения товара %s %v", merch, err)
			}
			//log.Println(store.Name)
			firstdeal := sales.Udate[0]
			lastdeal := sales.Udate[cnt-1]
			//sales.Prevdays частота продаж в днях, от предидущей покупки
			meanpd, sigmapd, stat := rbfnet.GetSigma(sales.Prevdays)
			meancnt, sigmacnt, statcnt := rbfnet.GetSigma(sales.Cnt)
			//если мало входных данных, то просто вычисляем по статистике
			if cnt < KC*2 {
				//если движение по складу было недавно, то закажем по последнему движению, иначе 0
				if now-lastdeal < 45 {
					//for i, z := cnt-1, 0; i >= 0 && z < 3; i-- {
					//	predict[z] = sales.Cnt[i]
					//	z++
					//}
					d := 0.0
					retNext = int(sigmapd)
					//если было только одно движение, то потребность считаем на количество дней от движения по текущию дату
					if len(sales.Prevdays) == 1 || sigmapd == 0 {
						//товар ыыодим на  рынок, ничего пока не известно
						if now-lastdeal > 0 {
							d = meancnt / (now - lastdeal)
							retNext = int(now - lastdeal)
						} else {
							d = meancnt / float64(PROGPER)
							retNext = PROGPER
						}
					} else {
						if meanpd != 0 {
							d = meancnt / meanpd
						} else {
							d = meancnt / float64(PROGPER)
							retNext = PROGPER
						}
					}
					retDemand = d
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
			nextdeal := int(predict[0])
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
			log.Printf("store=%v merch=%s %s, prognoz=%v  в следующие %v дней купят %v штук\n", store.Name, gt.Art, gt.Name, predict, nextdeal, p1)

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
				if sales.Cnt[k] < 0 { //возврат
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
				//если очень мало данных
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

//makeOrders делает таблицу заказов из predict для склада uidstorearg и товара uidgoodarg. Если склад и/или товар="", то для всех складов и всех товаров
func apiMakeOrders(uidstorearg, uidgoodarg string) {
	//var stores models.Stores
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
	MAXSALES := conf.ValInt("maxsales", 720)

	stores, err := models.GetMagNames(0, uidstorearg)

	if err != nil {
		models.DbLog("makeOrders. Ошибка чтения склада "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
		return
	}
	numzak := models.GetLastNumZakaz(now.Format("2006-01-02"))
	//по всем магазинам читаем матрицу и делаем заказ
	for _, store := range stores {
		uidstore := store.KeyStore
		tipmov := "S"
		if store.Tip == 0 {
			tipmov = "M"
		}
		var goods []models.MatrixGoods
		delivdays := 1
		//на основе данных по доставке делаем заказ
		var contracts []models.Contract
		contracts, err = models.GetContracts(uidstore)
		if err != nil {
			//несмогли прочитать контракты, ругаемся
			models.DbLog("makeOrders. Ошибка чтения контрактов "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
			return
		}

		//смотрим частоту заказов на склад
		delivdays = 1
		for _, contract := range contracts {
			if inChedule(contract.Chedord) {
				delivdays = contract.Delivdays
				provider = contract.Provider
				numzak++
				var ordersnum string
				//номер заказа два знака, всего 99 заказов в день
				if numzak < 10 {
					ordersnum = tipmov + strconv.Itoa(now.YearDay()) + "0" + strconv.Itoa(numzak)
				} else {
					ordersnum = tipmov + strconv.Itoa(now.YearDay()) + strconv.Itoa(numzak)
				}
				//если поставщик внешний, то у поставщика заказываем только по contractgoods ноиенклатуре поставщика
				isstore, err := models.GetMagNames(0, provider)
				//var provgoods map[string]models.Goods
				var outlineprovider bool = false
				if len(isstore) == 0 {
					//внешний поставщик
					outlineprovider = true
					delivdays = contract.Delivdays
					store.Tip = 9
					//provgoods, err = models.GetProviderGoods(provider, uidgoodarg)
				}
				switch store.Tip {
				case 1:
					//читаем матрицу, заказы делаем только по матрице
					goods, err = models.GetAllGoodsFromMatrix(uidstore, uidgoodarg)
				case 0:
					goods, err = models.GetOptMatrix(uidstore, uidgoodarg, MAXSALES)
				case 9:
					goods, err = models.GetCenterMatrix(uidgoodarg, provider)
				}

				if err != nil {
					models.DbLog("makeOrders. Ошибка чтения  матрицы магазина "+store.Name+" "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
					return
				}

				datedelivdays := now.AddDate(0, 0, delivdays+1).Format("2006-01-02")
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
					//если внешний поставщик, то есть ли этот товар у него?
					/*
						if outlineprovider {
							_, ok := provgoods[merch.KeyGoods]
							if !ok {
								//товара нет, следующий
								continue
							}
						}
					*/
					/*
						lp, err := models.GetLastPredict(uidstore, merch.KeyGoods)
						if err != nil {
							models.DbLog("makeOrders. Ошибка чтения таблицы предсказаний "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
							return
						}
					*/
					//если товар заказан уже то не заказываем
					//это условие проверим при записи в заказ
					//если остаток меньше чем минимум в матрице то доставляем до минималки

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
					recalc := false
					if merch.PredDays == 0 || (merch.PredDays > 0 && merch.PredDemand > (merch.PredCnt/float64(merch.PredDays))*10) {
						//ошибка сети, пересчитаем
						recalc = true
					}
					//для поставки на центральный склад статистика сборная по всем магазинам
					//уже собрана в goods = models.GetCenterMatrix
					if !outlineprovider && (recalc || now.Unix() > next.Unix()) {
						merch.PredDays, merch.PredDemand = calcnet(uidstore, merch.KeyGoods)
					}
					salestat, _ := models.GetSaleStat(uidstore, merch.KeyGoods, delivdays)
					if salestat["demand"] > 0 {
						if (merch.PredDemand > 1 || merch.PredDemand < 0) && (merch.PredDemand/salestat["demand"] > 2.0 || merch.PredDemand/salestat["demand"] < 2.0) {
							merch.PredDemand = salestat["demand"]
						}
					}
					//надо заказать для склада
					cntzak := float64(delivdays) * merch.PredDemand
					//смотрим текущий остаток и минимальный остаток склада
					//cntzak = cntzak - (merch.Balance - (merch.MinBalance + merch.Vitrina))
					//if cntzak+merch.Balance > merch.MaxBalance {
					//	cntzak = merch.MaxBalance - merch.Balance
					//}
					cntzak = cntzak - (merch.Balance - merch.Vitrina)
					//если указан максимальный баланс, то добиваем до него

					if merch.MinBalance > 0 && merch.Balance < merch.MinBalance {
						if cntzak < merch.MinBalance-merch.Balance {
							cntzak = merch.MinBalance - merch.Balance
						}
					}
					//но не более maxbalance
					if cntzak > 0.0 && cntzak+merch.Balance > merch.MaxBalance && merch.MaxBalance > 0 {
						cntzak = merch.MaxBalance - merch.Balance
					}
					//надо заказывать кратно step
					if cntzak > 0.0 && merch.Step > 1 {
						cntzak = float64(int(merch.Step) * int(cntzak/merch.Step+0.9999))
					}

					if cntzak > 0.0 && (int)(cntzak+0.5) > 0 {
						models.SaveOper(ordersnum, provider, uidstore, merch.KeyGoods, now.Format("2006-01-02"), cntzak, next.Format("2006-01-02"), datedelivdays)
					}
					//обновим ср потребность в матрице
					var m map[string]interface{}
					m = map[string]interface{}{}
					m["demand"] = merch.PredDemand
					w := make(map[string]string)
					w["uidStore"] = uidstore
					w["uidGoods"] = merch.KeyGoods
					models.UpdateMatrix(m, w)
				} //по товарам
			} //если стоит в расписании
		} //по контрактам
	} //по складам
	models.DbLog("end makeOrders. Конец составления заказов", "makeOrders", time.Now().UTC().UnixNano())
}

/*
//makeOrdersOpt делает таблицу заказов для оптовых складов
func apiMakeOrdersOpt() {
	//var stores *models.Stores
	var err error
	var now = time.Now() //сегодня
	var provider string

	Fop.Set(time.Now().UTC().UnixNano())
	err = models.DbLog("beg makeOrders. Начало составления заказов для оптовых складов", "makeOrders", Fop.Val())

	conf, err := models.GetConfig()
	MINDAYSORD := conf.ValInt("minDaysOrd", 7)
	MAXSALES := conf.ValInt("maxsales", 720)
	//только оптовые склады, тип +100=0
	stores, err := models.GetMagNames(-100, "")

	if err != nil {
		models.DbLog("makeOrders. Ошибка чтения склада "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
		return
	}
	numzak := 0
	//по всем складам делаем заказ
	for _, store := range stores {
		uidstore := store.KeyStore
		isneedord := false
		delivdays := 1
		var goods []models.MatrixGoods
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
		//номер заказа два знака, всего 99 заказов в день
		if numzak < 10 {
			ordersnum = "M" + strconv.Itoa(now.YearDay()) + "0" + strconv.Itoa(numzak)
		} else {
			ordersnum = "M" + strconv.Itoa(now.YearDay()) + strconv.Itoa(numzak)
		}
		//для розничного склада заказы делаем только по матрице, для оптового - статистике движений
		if store.Tip > 0 {
			goods, err = models.GetAllGoodsFromMatrix(uidstore, "")
		} else {
			goods, err = models.GetOptMatrix(uidstore, "", MAXSALES)
		}
		if err != nil {
			models.DbLog("makeOrders. Ошибка чтения  матрицы магазина "+store.Name+" "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
			return
		}
		datedelivdays := now.AddDate(0, 0, delivdays+1).Format("2006-01-02")
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

			//	lp, err := models.GetLastPredict(uidstore, merch.KeyGoods)
			//	if err != nil {
			//		models.DbLog("makeOrders. Ошибка чтения таблицы предсказаний "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
			//		return
			//	}
			//если товар заказан уже то не заказываем
			//это условие проверим при записи в заказ
			//если остаток меньше чем минимум в матрице то доставляем до минималки

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
			recalc := false
			if merch.PredDays > 0 && merch.PredDemand > (merch.PredCnt/float64(merch.PredDays))*20 {
				//ошибка сети, пересчитаем
				recalc = true
			}
			if recalc || now.Unix() > next.Unix() {
				merch.PredDays, merch.PredDemand = calcnet(uidstore, merch.KeyGoods)
			}
			//надо заказать для склада
			cntzak := float64(delivdays) * merch.PredDemand
			//смотрим текущий остаток и минимальный остаток склада
			//cntzak = cntzak - (merch.Balance - (merch.MinBalance + merch.Vitrina))
			//if cntzak+merch.Balance > merch.MaxBalance {
			//	cntzak = merch.MaxBalance - merch.Balance
			//}
			cntzak = cntzak - (merch.Balance - merch.Vitrina)
			if merch.MinBalance > 0 && merch.Balance < merch.MinBalance {
				if cntzak < merch.MinBalance-merch.Balance {
					cntzak = merch.MinBalance - merch.Balance
				}
			}

			//	if cntzak > 0.0 && cntzak < merch.MinBalance {
			//		cntzak = merch.MinBalance
			//	}

			if cntzak > 0.0 && cntzak+merch.Balance > merch.MaxBalance && merch.MaxBalance > 0 {
				cntzak = merch.MaxBalance - merch.Balance
			}
			//надо заказывать кратно step
			if cntzak > 0.0 && merch.Step > 1 {
				cntzak = float64(int(merch.Step) * int(cntzak/merch.Step+0.9999))
			}

			if cntzak > 0.0 && (int)(cntzak+0.5) > 0 {
				models.SaveOper(ordersnum, provider, uidstore, merch.KeyGoods, now.Format("2006-01-02"), cntzak, next.Format("2006-01-02"), datedelivdays)
			}
			//обновим ср потребность в матрице
			var m map[string]interface{}
			m = map[string]interface{}{}
			m["demand"] = merch.PredDemand
			w := make(map[string]string)
			w["uidStore"] = uidstore
			w["uidGoods"] = merch.KeyGoods
			models.UpdateMatrix(m, w)
		}
	}
	models.DbLog("end makeOrders. Конец составления заказов", "makeOrders", time.Now().UTC().UnixNano())
}
*/

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
