package main

import (
	"1c8_zak/models"
	"1c8_zak/rbfnet"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

//calcnet прогнозирунт продажи для склада store и товара kGoods.
func calcnet(store *models.Store, merch *models.MatrixGoods) map[string]float64 {
	var KC, INF, MAXSALES int
	//var stores []models.Store
	var guess = make([]float64, 3)
	var predict = make([]float64, 3)
	//now := int64(time.Now().Unix() / 86400) //сегодня

	var retmap = make(map[string]float64)

	Mc.Set(time.Now().UTC().UnixNano())
	defer func() {
		if r := recover(); r != nil {
			models.DbLog("err. конец составления прогноза по recover "+r.(error).Error(), "calculate", time.Now().UTC().UnixNano())
			log.Printf("err. конец составления прогноза по recover")
		}
	}()
	defer Mc.Set(0)

	conf, err := models.GetConfig()
	//PROGPER = conf.ValInt("minday2sale", 7)
	//KC коэф сети, соотношение вх нейронов к скрытым, для редких продаж лучше болье
	KC = conf.ValInt("kfnunvisible", 4)
	//INF удаленнность от последнего значения, см использование
	INF = conf.ValInt("inf", 30)
	//MAXSALES глубина просмотра продаж для составления статистики
	MAXSALES = conf.ValInt("maxsales", 720)
	//lsigma начальная сигма сети
	var lsigma float64 = 3
	//contracts, err = models.GetContracts()

	if Mc.Val() < 0 {
		//надо завершиться
		Mc.Set(0)
		models.DbLog("stop. завершено по сигналу", "calculate", time.Now().UTC().UnixNano())
		log.Printf("stop. завершено по сигналу")
		return retmap
	}
	sales, err := models.GetFNSales(store.KeyStore, merch.KeyGoods, time.Now().AddDate(0, 0, -MAXSALES).Format("2006-01-02"), time.Now().Format("2006-01-02"), store.Calendar)
	if err != nil {
		models.DbLog("calc. Ошибка чтения продаж магазина "+store.Name+" "+err.Error(), "calculate", time.Now().UTC().UnixNano())
		return retmap
		//log.Fatalf("Ошибка чтения продаж магазина %s %v", store.Name, err)
	}

	statmedian := sales.Stat[models.MedianCnt]
	statmediandays := sales.Stat[models.MedianDays]
	statsigmacnt := sales.Stat[models.SigmaCnt]
	statsigmadays := sales.Stat[models.SigmaDays]
	statmaxdays := sales.Stat[models.MaxDays]
	//satatmaxcnt := sales.Stat[models.MaxCnt]

	//если мало входных данных, то просто вычисляем по статистике
	if len(sales.Sdata) < 10 {
		return sales.Stat
	}
	//входные данные сети
	lendata := len(sales.Sdata)
	inputs := make([]float64, lendata+1)
	outputs := make([]float64, lendata+1)
	//прогноз по частоте покупок и по количеству
	for i := 0; i < lendata; i++ {
		//sales.Udate это номер дня Unix-число. Приводим к виду 1,2,3,4...
		inputs[i] = float64(i + 1)
		//нормализуем
		outputs[i] = float64(sales.Sdata[i].Wdays) / statmaxdays
	}
	guess[0] = inputs[lendata-1] + 1.0
	guess[1] = inputs[lendata-1] + 2.0
	guess[2] = inputs[lendata-1] + 3.0
	//v3-> привяжем в бесконечности к середине meanpd, чтобы функция не уползала в бесконечность
	if lendata < 30 {
		inputs[lendata] = guess[2] + float64(lendata)
	} else {
		inputs[lendata] = guess[2] + float64(INF)

		KC = int(math.Max(float64(KC), 14/statmediandays))
	}

	outputs[lendata] = statmediandays / statmaxdays
	// <-v3
	//если входных данных много, то часть берем для обучения, а три последних для проверки
	//Стаптистика по периодичности продаж

	if statsigmadays > 0 {
		lsigma = statsigmadays
	}
	centers := rbfnet.MakeCenters(inputs[0:lendata], KC)
	centers = append(centers, inputs[lendata])
	r := rbfnet.NewRBFNetwork(len(inputs), len(centers), lsigma, centers)
	errtrain := r.Train(inputs, outputs, 10000)
	//предсказания для guess
	copy(predict, r.Predict(guess))
	if errtrain < 0 {
		predict[0] = outputs[lendata]
		predict[1] = outputs[lendata]
	}
	//если predict[0] больше мин макс входных значений на 30% переучиваем сеть уменьшая количество скрытых нейронов

	//nextdeal следующая покупка будет через (денормализуем)
	nextdeal := predict[0] * statmaxdays
	//если nextdeal<среднего, то тренд на увеличение частоты покупок, если больше, то тренд на уменьшение
	errtrain = math.Sqrt(errtrain/float64(len(outputs))) * statmaxdays
	if nextdeal > 0 && errtrain < statsigmadays && errtrain > 0 {
		if math.Abs(nextdeal-statmediandays)/statmediandays < 0.3 {
			sales.Stat[models.MedianDays] = nextdeal
		}
	} else {
		if nextdeal < statmediandays {
			sales.Stat[models.MedianDays] = statmediandays * 0.8
		} else {
			sales.Stat[models.MedianDays] = statmediandays * 1.2
		}
	}
	//теперь прогноз количества следующей покупки
	p1 := 1.0
	//если разброс от центра маленький, то ничего считать не будем, берем центр
	if statsigmacnt < 3 {
		p1 = statmedian
	} else {
		centers = rbfnet.MakeCenters(inputs[0:lendata], KC)
		centers = append(centers, inputs[lendata])
		for i := 0; i < lendata; i++ {
			outputs[i] = float64(sales.Sdata[i].Cnt)
		}
		outputs[lendata] = statmedian

		if statsigmacnt > 0 {
			lsigma = statsigmacnt
		}
		r = rbfnet.NewRBFNetwork(len(inputs), len(centers), lsigma, centers)
		errtrain := r.Train(inputs, outputs, 10000)

		//log.Printf("ошика сети %v\n", o)
		copy(predict, r.Predict(guess))
		p1 = predict[0]
		if predict[0] < 0 {
			p1 = sales.Stat[models.MedianCnt]
		}
		errtrain = math.Sqrt(errtrain/float64(len(outputs))) * statmaxdays
		if p1 > 0 && errtrain < statsigmacnt {
			if math.Abs(p1-statmedian)/statmedian < 0.3 {
				sales.Stat[models.MedianCnt] = p1
			}
		}
	}
	log.Printf("store=%v merch=%s prognoz=%v в следующие %v дней купят %v штук\n", store.Name, merch.KeyGoods, predict, nextdeal, p1)
	//запишем матрицу
	merch.Sigmacnt = sales.Stat[models.SigmaCnt]
	merch.Sigmadays = sales.Stat[models.SigmaDays]
	merch.PredCnt = sales.Stat[models.MedianCnt]
	merch.PredDays = sales.Stat[models.MedianDays]
	merch.PredDemand = merch.PredCnt / merch.PredDays
	merch.PredPeriod = time.Now().Format("2006-01-02")
	merch.SavePredict(store.KeyStore, sales.LastSum, sales.LastMargin)
	return sales.Stat
	/*
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
		if statcnt["mean"] > 0 && stat["mean"] > 0 {
			demand[cnt] = statcnt["mean"] / stat["mean"]
		}
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
	*/
	//break

}

//apiMakeDistributionOrders делает таблицу заказов  для распределительного склада
func apiMakeDistributionOrders(distrstore *models.Store, contract *models.Contract, uidgoodarg string, numzak int) {
	//соберет все остатки, кроме складов, которых нет в матрице
	provider := contract.Provider
	goods, err := models.GetCenterMatrix(provider, uidgoodarg)
	if err != nil {
		models.DbLog("makeOrders. Ошибка чтения  матрицы магазина "+distrstore.Name+" "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
		return
	}
	models.DbLog("makeOrders. расчет заказов магазина "+distrstore.Name+" поставщик "+contract.ProviderName, "makeOrders", time.Now().UTC().UnixNano())

	//datedelivdays дата доставки товара
	datedelivdays := time.Now().AddDate(0, 0, contract.Delivdays).Format("2006-01-02")
	//собираем для каждого товара заказы по магазинам
	goodszak := make(map[string]float64)
	store := new(models.Store)
	//для каждого магазина смотрим потребность
	for _, merch := range goods {
		//прибавляем для округления
		round := 0.5
		if Fop.Val() < 0 {
			//надо завершиться
			Fop.Set(0)
			models.DbLog("stop. завершено по сигналу", "makeOrders", time.Now().UTC().UnixNano())
			log.Printf("stop. завершено по сигналу")
			return
		}
		//store текущий магазин
		store.Get(merch.KeyStore)
		//надо ли пересчистать статистику?
		recalc := false
		//prper дата предсказания
		prper, err := time.Parse("2006-01-02", merch.PredPeriod)
		if err != nil {
			prper = time.Now().AddDate(0, 0, -100)
		}
		//lper дата последней продажи товара
		/*
			lper, err := time.Parse("2006-01-02", merch.Lastdeal)
			if err != nil {
				//нет даты прогноза или формат даты другой
				lper, err = time.Parse("2006-01-02T15:04:05", merch.Lastdeal)
				if err != nil {
					lper = time.Now()
				}
			}
		*/
		//next с вероятностью более 95% сделка следующая будет до даты
		//next := lper.AddDate(0, 0, int(merch.PredDays+2*merch.Sigmadays))
		//nextcnt количество, которое необходимо обеспечить до даты next, по сути расчетный мин остаток склада
		nextcnt := merch.PredCnt + merch.Sigmacnt

		//раз в 14 дней пересчет статистики
		nextrecalc := 3 * merch.Sigmadays
		if nextrecalc < 14 {
			nextrecalc = 14
		}
		if nextrecalc > 30 {
			nextrecalc = 30
		}
		if merch.PredDays == 0 || merch.Sigmadays == 0 || time.Now().Sub(prper).Hours()/24 > nextrecalc {
			recalc = true
		}

		//для поставки на центральный склад статистика сборная по всем магазинам
		//уже собрана в goods = models.GetCenterMatrix
		if recalc {
			stat := calcnet(store, &merch)
			merch.PredDays = stat[models.MedianDays]
			merch.PredCnt = stat[models.MedianCnt]
			merch.Sigmacnt = stat[models.SigmaCnt]
			merch.Sigmadays = stat[models.SigmaDays]
			if merch.PredDays > 0 {
				merch.PredDemand = merch.PredCnt / merch.PredDays
			}
			nextcnt = merch.PredCnt + merch.Sigmacnt
		}
		//
		if contract.Delivdays > int(merch.PredDays+merch.Sigmadays) {
			nextcnt = nextcnt / (merch.PredDays + merch.Sigmadays) * float64(contract.Delivdays)
		}
		//надо заказать для склада
		cntzak := float64(contract.Delivdays) * merch.PredDemand
		//округлим cntzak
		cntzak = float64(int64(cntzak + 0.99))
		//explain := `"days":` + strconv.FormatInt(int64(delivdays), 10) + `,"demand":` + strconv.FormatFloat(merch.PredDemand, 'f', 10, 64) + `,"z1":` + strconv.FormatFloat(cntzak, 'f', 0, 64)
		if cntzak < nextcnt {
			cntzak = nextcnt
			//explain = explain + `",zstat":` + strconv.FormatFloat(nextcnt, 'f', 0, 64)
		}
		//пока товар едет может продаться штук
		salecnt := float64(contract.Delivdays) * merch.PredDemand
		if salecnt < 0.1 {
			salecnt = 0.0
		}
		salecnt = float64(int64(salecnt + 0.99))
		//к моменту доставки на складе останется
		balance := merch.Balance - salecnt
		if balance < 0 {
			balance = 0
		}
		cntzak = cntzak - (balance - merch.Vitrina)
		//explain = explain + ",\"curbalance\":" + strconv.FormatFloat(merch.Balance, 'f', 2, 64) + ",\"delivdays\":" + strconv.FormatInt(int64(contract.Delivdays), 10) + ",\"delivsales\":" + strconv.FormatFloat(salecnt, 'f', 0, 64) + ",\"delivbalance\":" + strconv.FormatFloat(balance, 'f', 3, 64) +
		//	",\"myminbalance\":" + strconv.FormatFloat(nextcnt, 'f', 2, 64) + ",\"minbalance\":" + strconv.FormatFloat(merch.MinBalance, 'f', 2, 64)
		//если указан максимальный баланс, то добиваем до него
		if merch.MinBalance > 0 && balance < merch.MinBalance {
			if cntzak < merch.MinBalance-balance {
				cntzak = merch.MinBalance - balance
				//explain = explain + ",\"zforminbalance\":" + strconv.FormatFloat(cntzak, 'f', 2, 64)
			}
		}
		//if cntzak > 0.0 && cntzak+balance > merch.MaxBalance && merch.MaxBalance > 0 {
		//но не более maxbalance
		if cntzak > 0 && cntzak+merch.Balance > merch.MaxBalance && merch.MaxBalance > 0 {
			cntzak = merch.MaxBalance - merch.Balance
			//explain = explain + ",\"maxbalance\":" + strconv.FormatFloat(merch.MaxBalance, 'f', 2, 64) + ",\"zformaxbalance\":" + strconv.FormatFloat(cntzak, 'f', 2, 64)
		}
		//надо заказывать кратно step
		if (int)(cntzak+0.5) > 0 && merch.Step > 1 {
			cntzak = float64(int(merch.Step) * int(cntzak/merch.Step+0.9999))
			//explain = explain + ",\"step\":" + strconv.FormatFloat(merch.Step, 'f', 2, 64) + ",\"zstep\":" + strconv.FormatFloat(cntzak, 'f', 2, 64)
		}

		if cntzak > 0.0 && (int)(cntzak+round) > 0 {
			goodszak[merch.KeyGoods] = goodszak[merch.KeyGoods] + cntzak
			//models.SaveOper(ordersnum, provider, uidstore, merch.KeyGoods, now.Format("2006-01-02"), float64((int)(cntzak+round)), next.Format("2006-01-02"), datedelivdays, explain)
		}
	} //по товарам
	//теперь запишем в базу
	var ordersnum string
	//номер заказа два знака, всего 99 заказов в день
	if numzak < 10 {
		ordersnum = strconv.Itoa(time.Now().Year()-2000) + strconv.Itoa(time.Now().YearDay()) + "0" + strconv.Itoa(numzak)
	} else {
		ordersnum = strconv.Itoa(time.Now().Year()-2000) + strconv.Itoa(time.Now().YearDay()) + strconv.Itoa(numzak)
	}
	for goodskey, itzak := range goodszak {
		//остаток центрального склада
		_, lb, _ := models.GetLastBalance(distrstore.KeyStore, goodskey)
		itzak = itzak - lb
		models.SaveOper(ordersnum, provider, distrstore.KeyStore, goodskey, time.Now().Format("2006-01-02"), float64((int)(itzak)), datedelivdays, "")
	}
	return
}

//makeOrders делает таблицу заказов из predict для склада uidstorearg и товара uidgoodarg. Если склад и/или товар="", то для всех складов и всех товаров
func apiMakeOrders(uidstorearg, uidgoodarg string) {
	//var stores models.Stores
	var err error
	var now = time.Now() //сегодня
	var provider string

	if Fop.Val() > 0 {
		//уже считаем
		return
	}
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
	//если продажи реже, чем одна штука в DAYSNOSALE, то рекомендуем товар как заказной, тип F в ABC классификации
	//DAYSNOSALE := conf.ValInt("daysnosale", 45)
	//RECALCMIN минимальное количество дней через которые идет пересчет статистики
	RECALCMIN := conf.ValInt("recalcmin", 7)
	//RECALCMAX максимальное количество дней через которые идет пересчет статистики
	RECALCMAX := conf.ValInt("recalcmax", 30)
	numzak := models.GetLastNumZakaz(now.Format("2006-01-02"))
	//читаем все контракты

	delivdays := 1
	//на основе данных по доставке делаем заказ
	var contracts []models.Contract
	contracts, err = models.GetContracts(uidstorearg)
	if err != nil {
		//несмогли прочитать контракты, ругаемся
		models.DbLog("makeOrders. Ошибка чтения контрактов "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
		return
	}
	for _, contract := range contracts {
		inched := true
		if strings.Contains(contract.Chedord, "/") {
			t, err := models.GetLastOrd(contract.Provider)
			if err == nil {
				inched = inChedule(contract.Chedord, time.Now(), t)
			} else {
				inched = inChedule(contract.Chedord)
			}
		} else {
			inched = inChedule(contract.Chedord)
		}
		if inched {
			//tipmov := "S"
			delivdays = contract.Delivdays
			provider = contract.Provider
			store := new(models.Store)
			//заполним store
			store.Get(contract.Recipient)
			if store.Tip == models.NotUsed {
				continue
			}
			//if store.Tip == models.Distribution || store.Tip == models.Retail {
			//	tipmov = "M"
			//}
			numzak++
			var ordersnum string
			//номер заказа два знака, всего 99 заказов в день
			if numzak < 10 {
				ordersnum = strconv.Itoa(time.Now().Year()-2000) + strconv.Itoa(now.YearDay()) + "0" + strconv.Itoa(numzak)
			} else {
				ordersnum = strconv.Itoa(time.Now().Year()-2000) + strconv.Itoa(now.YearDay()) + strconv.Itoa(numzak)
			}
			//если поставщик внешний, то у поставщика заказываем только по contractgoods ноиенклатуре поставщика
			//provstore поставщик
			provstore := new(models.Store)
			provstore.Get(contract.Provider)
			var outlineprovider bool = false
			//provstore не заполнен, поставщик не в списке складов, хначит он внешний
			if provstore.KeyStore == "" {
				//внешний поставщик
				outlineprovider = true
				delivdays = contract.Delivdays
			}
			var goods []models.MatrixGoods
			uidstore := store.KeyStore
			switch {
			case store.Tip >= models.SaleXL:
				//читаем матрицу, заказы делаем только по матрице
				goods, err = models.GetAllGoodsFromMatrix(uidstore, uidgoodarg)
			case store.Tip == models.Retail:
				goods, err = models.GetOptMatrix(uidstore, uidgoodarg, MAXSALES)
			case store.Tip == models.Distribution:
				//соберет все остатки, кроме складов, которых нет в матрице
				//goods, err = models.GetCenterMatrix(provider, uidgoodarg)
				apiMakeDistributionOrders(store, &contract, uidgoodarg, numzak)
				continue
			}

			if err != nil {
				models.DbLog("makeOrders. Ошибка чтения  матрицы магазина "+store.Name+" "+err.Error(), "makeOrders", time.Now().UTC().UnixNano())
				return
			}
			models.DbLog("makeOrders. расчет заказов магазина "+store.Name+" поставщик "+contract.ProviderName, "makeOrders", time.Now().UTC().UnixNano())
			//datedelivdays дата доставки товара
			datedelivdays := now.AddDate(0, 0, delivdays).Format("2006-01-02")
			if delivdays < MINDAYSORD {
				delivdays = MINDAYSORD
			}
			for _, merch := range goods {
				//прибавляем для округления
				round := 0.5
				if Fop.Val() < 0 {
					//надо завершиться
					Fop.Set(0)
					models.DbLog("stop. завершено по сигналу", "makeOrders", time.Now().UTC().UnixNano())
					log.Printf("stop. завершено по сигналу")
					return
				}
				//если товар заказан уже то не заказываем
				//это условие проверим при записи в заказ
				//если остаток меньше чем минимум в матрице то доставляем до минималки

				//надо ли пересчистать статистику?
				recalc := false
				//prper дата предсказания
				prper, err := time.Parse("2006-01-02", merch.PredPeriod)
				if err != nil {
					prper = time.Now().AddDate(0, 0, -100)
				}
				//lper дата последней продажи товара
				/*
					lper, err := time.Parse("2006-01-02", merch.Lastdeal)
						if err != nil {
							//нет даты прогноза или формат даты другой
							lper, err = time.Parse("2006-01-02T15:04:05", merch.Lastdeal)
							if err != nil {
								lper = time.Now()
							}
						}
					//next с вероятностью более 95% сделка следующая будет до даты
					next := lper.AddDate(0, 0, int(merch.PredDays+2*merch.Sigmadays))
				*/
				//nextcnt количество, которое необходимо обеспечить до даты next, по сути расчетный мин остаток склада
				nextcnt := merch.PredCnt + 2*merch.Sigmacnt
				//minost рекомендуемый минимальный остаток склада
				minost := int(nextcnt/(merch.PredDays+2*merch.Sigmadays)*float64(contract.Delivdays) + 0.5)
				explain := `"minost":` + strconv.FormatInt(int64(minost), 10)
				//раз в 14 дней пересчет статистики
				nextrecalc := int(3 * merch.Sigmadays)
				if nextrecalc < RECALCMIN {
					nextrecalc = RECALCMIN
				}
				if nextrecalc > RECALCMAX {
					nextrecalc = RECALCMAX
				}
				if merch.PredDays == 0 || merch.Sigmadays == 0 || now.Sub(prper).Hours()/24 > float64(nextrecalc) {
					recalc = true
				}
				//для поставки на центральный склад статистика сборная по всем магазинам
				//уже собрана в goods = models.GetCenterMatrix
				if recalc {
					stat := calcnet(store, &merch)
					merch.PredDays = stat[models.MedianDays]
					merch.PredCnt = stat[models.MedianCnt]
					merch.Sigmacnt = stat[models.SigmaCnt]
					merch.Sigmadays = stat[models.SigmaDays]
					if merch.PredDays > 0 {
						merch.PredDemand = merch.PredCnt / merch.PredDays
					}
					nextcnt = merch.PredCnt + merch.Sigmacnt
				}
				//
				if contract.Delivdays > int(merch.PredDays+2*merch.Sigmadays) {
					nextcnt = nextcnt / (merch.PredDays + 2*merch.Sigmadays) * float64(contract.Delivdays)
				}
				/*
					if !outlineprovider {
						//if merch.PredDemand <= 0 || merch.PredDemand > 0.3 {
						salestat, _ := models.GetSaleStat(uidstore, merch.KeyGoods, DAYSNOSALE)
						if salestat["demand"] > 0 && (merch.PredDemand/salestat["demand"] > 1.5 || merch.PredDemand/salestat["demand"] < 0.67) {
							merch.PredDemand = salestat["demand"]
							if salestat["demand"] < 0.001 || salestat["deals"] == 0 {
								//надо перевести в категорию F
								merch.Abc = "F"
							}
						}
						//}
					} else {
						//для центрального склада добавим его остаток, в nerch остатки только по матрице
						_, lb, _ := models.GetLastBalance("", merch.KeyGoods)
						if lb > 0 {
							merch.Balance = lb
						}
						//if merch.PredDemand <= 0 || merch.PredDemand > 1 {
						salestat, _ := models.GetSaleStat("", merch.KeyGoods, DAYSNOSALE)
						if salestat["demand"] > 0 && (merch.PredDemand/salestat["demand"] > 1.3 || merch.PredDemand/salestat["demand"] < 0.77) {
							merch.PredDemand = salestat["demand"]
						}
						round = 0.999
						//}
					}
				*/
				//надо заказать для склада
				cntzak := float64(delivdays) * merch.PredDemand
				//округлим cntzak
				cntzak = float64(int64(cntzak + 0.99))
				explain = explain + `,"days":` + strconv.FormatInt(int64(delivdays), 10) + `,"demand":` + strconv.FormatFloat(merch.PredDemand, 'f', 10, 64) + `,"z1":` + strconv.FormatFloat(cntzak, 'f', 0, 64)
				if cntzak < nextcnt && math.Abs(cntzak-nextcnt)/cntzak < 0.3 {
					cntzak = nextcnt
					explain = explain + `,"zstat":` + strconv.FormatFloat(nextcnt, 'f', 1, 64)
				}
				//смотрим текущий остаток и минимальный остаток склада
				//cntzak = cntzak - (merch.Balance - (merch.MinBalance + merch.Vitrina))
				//if cntzak+merch.Balance > merch.MaxBalance {
				//	cntzak = merch.MaxBalance - merch.Balance
				//}
				//пока товар едет может продаться штук
				salecnt := float64(contract.Delivdays) * merch.PredDemand
				if salecnt < 0.1 {
					salecnt = 0.0
				}
				salecnt = float64(int64(salecnt + 0.99))
				//к моменту доставки на складе останется
				balance := merch.Balance - salecnt
				if balance < 0 {
					balance = 0
				}
				cntzak = cntzak - (balance - merch.Vitrina)
				explain = explain + `,"curbalance":` + strconv.FormatFloat(merch.Balance, 'f', 2, 64) + `,"delivdays":` + strconv.FormatInt(int64(contract.Delivdays), 10) + `,"delivsales":` + strconv.FormatFloat(salecnt, 'f', 0, 64) + `,"delivbalance":` + strconv.FormatFloat(balance, 'f', 3, 64) +
					`,"minbalance":` + strconv.FormatFloat(merch.MinBalance, 'f', 2, 64)
				//если указан максимальный баланс, то добиваем до него
				if merch.MinBalance > 0 && balance < merch.MinBalance {
					if cntzak < merch.MinBalance-balance {
						cntzak = merch.MinBalance - balance
						explain = explain + `,"zforminbalance":` + strconv.FormatFloat(cntzak, 'f', 2, 64)
					}
				}
				//if cntzak > 0.0 && cntzak+balance > merch.MaxBalance && merch.MaxBalance > 0 {
				//но не более maxbalance
				if cntzak > 0 && cntzak+merch.Balance > merch.MaxBalance && merch.MaxBalance > 0 {
					cntzak = merch.MaxBalance - merch.Balance
					explain = explain + `,"maxbalance":` + strconv.FormatFloat(merch.MaxBalance, 'f', 2, 64) + `,"zformaxbalance":` + strconv.FormatFloat(cntzak, 'f', 2, 64)
				}
				//надо заказывать кратно step
				if (int)(cntzak+0.5) > 0 && merch.Step > 1 {
					cntzak = float64(int(merch.Step) * int(cntzak/merch.Step+0.9999))
					explain = explain + `,"step":` + strconv.FormatFloat(merch.Step, 'f', 2, 64) + `,"zstep":` + strconv.FormatFloat(cntzak, 'f', 2, 64)
				}

				if cntzak > 0.0 && (int)(cntzak+round) > 0 {
					//если у поставщика нет достаточного количества, то подбираем аналог
					if !outlineprovider {
						_, provbalance, _ := models.GetLastBalance(provider, merch.KeyGoods)
						if provbalance < cntzak {
							analog, ost, _ := models.GetAnalog(provider, merch.KeyGoods)
							if len(analog) > 0 {
								var gds *models.Goods
								if ost > 0 {
									gds, _ = models.GetGood(merch.KeyGoods)
								} else {
									gds, _ = models.GetGood(analog)
								}
								gname := gds.Name
								if len(gname) > 0 {
									gname = gds.Name + " (" + gds.Art + ")"
								} else {
									gname = merch.KeyGoods
								}
								explain = explain + `,"analog":"` + gname + `","anbalance":` + strconv.FormatFloat(ost, 'f', 2, 64)
								if ost > 0 {
									merch.KeyGoods = analog
								}
							}
						}
					}
					models.SaveOper(ordersnum, provider, uidstore, merch.KeyGoods, now.Format("2006-01-02"), float64((int)(cntzak+round)), datedelivdays, explain)
				}
				//обновим ср потребность в матрице
				var m map[string]interface{}
				m = map[string]interface{}{}
				m["demand"] = merch.PredDemand
				m["comment"] = explain
				if merch.Abc == "F" {
					m["abc"] = merch.Abc
				}
				w := make(map[string]string)
				w["uidStore"] = uidstore
				w["uidGoods"] = merch.KeyGoods
				models.UpdateMatrix(m, w)
			} //по товарам
		}
	} //по контрактам

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

//RecalcProfit статистика и прогноз финансовых показателей
func RecalcProfit(keystore, pfrom, pto string) (map[string][]models.ProfitGraph, float64, error) {
	//получим статистику по магазинам по месяцам
	if pfrom == "" {
		pfrom = "2006-01-02"
	}
	if pto == "" {
		pto = time.Now().Format("2006-01-02")
	}
	stat, err := models.GetProfitMounth(keystore, pfrom, pto)
	if err != nil {
		return stat, 1, err
	}
	retqerr := float64(0)
	//расчитаем прогноз по финансовым показателям на след три месяца для каждого магазина
	//последний месяц в расчет не берем, если он не полный
	for uidstore, magstat := range stat {
		lenst := len(magstat)
		if time.Now().Day() < 29 {
			lenst--
		}
		//lastper последний период
		var lastper string
		//profit по магазину прибыль
		profit := make([]float64, lenst+4)
		//proceed продажи
		proceed := make([]float64, lenst+4)
		//количества
		//cnts := make([]float64, lenst+3)
		inputs := make([]float64, lenst+4)
		centers := make([]float64, 0, 12)
		for k := 0; k < lenst; k++ {
			//profit = append(profit, float64(st.Profit))
			profit[k] = float64(magstat[k].Profit)
			proceed[k] = float64(magstat[k].Proceed)
			//cnts[k] = float64(magstat[k].Cnt)
			inputs[k] = float64(k + 1)
			m, _ := strconv.Atoi(magstat[k].Period[5:])
			if len(centers) == 0 && m > 8 && m < 11 {
				centers = append(centers, inputs[k])
			}
			if m == 8 || m == 2 { //центры ставим на февраль и август
				centers = append(centers, inputs[k])
			}
			//inputs = append(inputs, float64(k+1))
			//proceed = append(proceed, float64(st.Proceed))
			//cnts = append(cnts, float64(st.Cnt))
			lastper = magstat[k].Period
		}
		//добавим виртуальный период с медианным значением
		inputs[lenst] = float64(lenst + 4)
		mean, _, _ := rbfnet.StdDev(profit)
		if lenst < 12 {
			profit[lenst] = mean
		} else {
			profit[lenst] = profit[lenst-(12-4)]
		}
		mean, _, _ = rbfnet.StdDev(proceed)
		if lenst < 12 {
			proceed[lenst] = mean
		} else {
			proceed[lenst] = proceed[lenst-(12-4)]
		}
		year, month, _ := time.Now().Date()
		y, err := strconv.Atoi(lastper[0:4])
		if err != nil {
			y = year
		}
		m, err := strconv.Atoi(lastper[5:])
		if err != nil {
			m = int(month)
		}
		//заполним прогнозируемые периоды
		for k := 0; k < 3; k++ {
			m++
			if m > 12 {
				m = 1
				y++
			}
			stat[uidstore] = append(stat[uidstore], models.ProfitGraph{})
			if m > 9 {
				stat[uidstore][k+lenst].Period = strconv.FormatInt(int64(y), 10) + "-" + strconv.FormatInt(int64(m), 10)
			} else {
				stat[uidstore][k+lenst].Period = strconv.FormatInt(int64(y), 10) + "-0" + strconv.FormatInt(int64(m), 10)
			}
		}
		for k := lenst + 1; k < lenst+4; k++ {
			//inputs = append(inputs, float64(k+1))
			//profit = append(profit, float64(k+1))
			//proceed = append(proceed, float64(0))
			//cnts = append(cnts, float64(0))
			profit[k] = float64(0)
			proceed[k] = float64(0)
			//cnts[k] = float64(0)
			inputs[k] = float64(k)
		}

		_, _, statf := rbfnet.GetSigma(profit[:lenst])
		//предсказания продаж
		_, _, statp := rbfnet.GetSigma(proceed[:lenst])
		if lenst < 5 {
			for k := 0; k < 3; k++ {
				stat[uidstore][k+lenst].Profit = int64(statf["mean"] + 0.5)
				stat[uidstore][k+lenst].Proceed = int64(statp["mean"] + 0.5)
			}
			continue
		}
		//normalize
		for k, v := range profit {
			profit[k] = v / statf["max"]
		}
		//количество известных значений
		EXPL := 0
		//centers := rbfnet.MakeCenters(inputs[0:lenst-EXPL], 6)

		//if float64(lenst-EXPL-1)-centers[len(centers)-1] > 2 {
		//	centers = append(centers, centers[len(centers)-1]+6)
		//}
		if len(centers) == 0 {
			centers = append(centers, inputs[int(lenst/2)])
			//centers[0] = inputs[int(lenst/2)]
		} else {
			centers = append(centers, centers[len(centers)-1]+6)
		}
		r := rbfnet.NewRBFNetwork(lenst-EXPL+1, len(centers), 6, centers)

		//предсказания выручки
		//qerr := r.TrainW(inputs[0:lenst+1], profit[:lenst+1], EXPL, 1000)
		qerr := r.Train(inputs[0:lenst+1], profit[:lenst+1], 1000)
		copy(profit[lenst:], r.Predict(inputs[lenst+1:]))
		//получили тренд.
		for k := 0; k < 3; k++ {
			stat[uidstore][k+lenst].Profit = int64((profit[lenst+k]) * statf["max"])
		}

		//normalize
		for k, v := range proceed {
			proceed[k] = v / statp["max"]
		}
		//qerr = r.TrainW(inputs[0:lenst+1], proceed[:lenst+1], EXPL, 1000)
		qerr = r.Train(inputs[0:lenst+1], proceed[:lenst+1], 1000)
		copy(proceed[lenst:], r.Predict(inputs[lenst+1:]))
		for k := 0; k < 3; k++ {
			stat[uidstore][k+lenst].Proceed = int64((proceed[lenst+k]) * statp["max"])
		}
		if retqerr < math.Abs(qerr) {
			retqerr = qerr
		}

	}
	return stat, retqerr, nil
}
