package main

import (
	"encoding/xml"
	"flag"
	"log"
	"math"
	"net/http"
	"sort"

	"strconv"
	"sync"
	"time"

	"1c8_zak/models"

	"1c8_zak/rbfnet"

	"html/template"

	gintemplate "github.com/foolin/gin-template"
	"github.com/gin-gonic/gin"
)

//Mcalc флаг работы функции calculate
type Mcalc struct {
	mu sync.Mutex
	x  int64
}

//Set установка значения флага
func (c *Mcalc) Set(x int64) {
	c.mu.Lock()
	c.x = x
	c.mu.Unlock()
}

//Val получение значения флага
func (c *Mcalc) Val() (x int64) {
	c.mu.Lock()
	x = c.x
	c.mu.Unlock()
	return
}

//Mc флаг работы расчета
var Mc Mcalc

//Fop флаг работы горутины oper
var Fop Mcalc

//calculate прогнозирунт продажи, lkc-коф для сети, отношение входных нейронов к скрытым, progper-количество дней прогноза
func calculate(c *gin.Context) {
	//читаем состояние сети
	//если сеть в процессе расчета и нет сигнала остановки, то сообщаем информацию и выходим
	// a channel to tell it to stop

	stop := c.DefaultQuery("stop", "none")
	start := c.DefaultQuery("start", "none")
	store := c.DefaultQuery("store", "")
	goods := c.DefaultQuery("goods", "")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "stop= " + stop + " store=" + store + " goods=" + goods})
	if stop != "none" && stop != "" {
		v, err := strconv.Atoi(stop)
		if err == nil {
			x := Mc.Val()
			if int64(v) == x && x > 0 {
				Mc.Set(-1 * x)
				c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "seting Mc = " + strconv.FormatInt(Mc.Val(), 10)})
			} else {
				c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "bad stop signal. lookin for " + strconv.FormatInt(Mc.Val(), 10)})
			}
		} else {
			c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": " Atoi err = " + err.Error()})
		}
		return
	}
	if Mc.Val() != 0 {
		c.JSON(http.StatusLocked, gin.H{"status": http.StatusLocked, "start": time.Unix(0, Mc.Val()).Format("2 Jan 2006 15:04:05"), "message": "в процессе расчета " + strconv.FormatInt(Mc.Val(), 10) + " store=" + store + " goods=" + goods})
		return
	}
	if start == "ok" {
		/*
			slog := models.GetLastStateNetwork(1, "calculate")
			for _, v := range slog {
				if v[:3] != "end" {
					//находимся в состоянии расчета. выводим это
					c.JSON(http.StatusLocked, gin.H{"status": http.StatusLocked, "start": time.Unix(0, Mc.Val()).Format("2 Jan 2006 15:04:05"), "message": "в процессе расчета " + strconv.FormatInt(Mc.Val(), 10) + " store=" + store + " goods=" + goods})
					return
				}
			}*/
		go calcnet(store, goods)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "запущен расчет " + time.Now().Format("2 Jan 2006 15:04:05") + " store=" + store + " goods=" + goods})
		return
	}
	slog := models.GetLastStateNetwork(3, "calculate")
	status := make(map[string]string, 3)
	status["err"] = "no"
	for k, v := range slog {
		switch v[:3] {
		case "end":
			status["end"] = time.Unix(0, int64(k)).Format("2 Jan 2006 15:04:05") + v[4:]
		case "beg":
			status["beg"] = time.Unix(0, int64(k)).Format("2 Jan 2006 15:04:05") + v[4:]
		default:
			status["err"] = time.Unix(0, int64(k)).Format("2 Jan 2006 15:04:05") + v[4:]
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "beg": status["beg"], "err": status["err"], "end": status["end"]})
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
			gt, err := models.GetGoods(merch.KeyGoods)
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

func createGoods(c *gin.Context) {
	g := models.Goods{
		Name:     c.PostForm("name"),
		Art:      c.PostForm("art"),
		Grp:      c.PostForm("group"),
		KeyGoods: c.PostForm("uid"),
	}
	cnt, err := models.CreateGoods(&g)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "message": "Goods item created successfully!", "id": cnt})
}

func fetchAllStocks(c *gin.Context) {
	st, err := models.GetMagNames(-1, "")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
		return
	}
	if len(*st) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "No todo found!"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": *st})
}

func updateGoods(c *gin.Context) {
	id := c.Param("id")

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "Goods updated successfully!", "uid": id})
}

func fetchSingleGoods(c *gin.Context) {
	goodsuid := c.Param("id")
	gd, err := models.GetGoods(goodsuid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
		return
	}
	if len(gd.Art) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "No goods found!"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "art": gd.Art, "name": gd.Name, "group": gd.Grp})
}

func getNeuroData(c *gin.Context) {
	goodsuid := c.Param("goods")
	storeuid := c.Param("store")
	nr, err := models.LoadRbfNet(storeuid, goodsuid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
		return
	}
	c.JSON(http.StatusNotFound, nr)
}

func getPredict(c *gin.Context) {
	goodsuid := c.Param("goods")
	storeuid := c.Param("store")
	nr, err := models.GetLastPredict(storeuid, goodsuid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
		return
	}
	c.JSON(http.StatusNotFound, nr)
}

func setSales(c *gin.Context) {

	ids := c.DefaultQuery("id", "0")
	id, err := strconv.Atoi(ids)
	if err != nil {
		id = 0
	}

	type Sales struct {
		Uidstore   string  `json:"uidstore" binding:"required"`
		Uidgoods   string  `json:"uidgoods" binding:"required"`
		GroupGoods string  `json:"groupGoods"`
		Period     string  `json:"period" binding:"required"`
		Tipmov     string  `json:"tipmov" binding:"required"`
		Cnt        float64 `json:"cnt" binding:"required"`
		Summa      float64 `json:"summa" binding:"required"`
		Margin     float64 `json:"margin" binding:"required"`
		Balance    float64 `json:"balance" binding:"required"`
		Prevdays   string  `json:"prevd" binding:"required"`
		Zerodays   string  `json:"zerod" binding:"required"`
	}
	var sm []Sales
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}
	/*
		CREATE TABLE "goodsmov" (
			"id"	integer,
			"uidStore"	text NOT NULL,
			"uidGoods"	text NOT NULL,
			"groupGoods"	text,
			"period"	text NOT NULL,
			"cnt"	real,
			"summa"	integer,
			"margin"	real,
			"balance"	real,
			"prevdays"	integer,
			"zerodays"	integer,
			"tipmov"	TEXT DEFAULT 'S',
			PRIMARY KEY("id")
		)
	*/
	matr := make([]map[string]interface{}, 0, 256)
	for _, v := range sm {
		m := make(map[string]interface{})
		if id > 0 {
			m["id"] = id
		}
		m["uidStore"] = v.Uidstore
		m["uidGoods"] = v.Uidgoods
		m["period"] = v.Period
		m["tipmov"] = v.Tipmov
		m["groupGoods"] = v.GroupGoods
		m["cnt"] = 0
		m["summa"] = v.Summa
		m["margin"] = v.Margin
		m["balance"] = v.Balance
		m["prevdays"] = v.Prevdays
		m["zerodays"] = v.Zerodays
		matr = append(matr, m)
	}
	err = models.InsRepSales(matr)
	if err != nil {
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})

}

//makeOrders делает таблицу заказов из predict
func makeOrders() {
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
			lper, err := time.Parse("2006-01-02", merch.PredPeriod)
			if err != nil {
				//нет даты прогноза или формат даты другой
				lper = time.Unix(0, 0)
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

			if (int)(cntzak+0.5) > 0 {
				models.SaveOper(ordersnum, provider, uidstore, merch.KeyGoods, now.Format("2006-01-02"), cntzak, next.Format("2006-01-02"), datedelivdays)
			}
		}
	}
	models.DbLog("end makeOrders. Конец составления заказов", "makeOrders", time.Now().UTC().UnixNano())
}

//mkorders пишет в таблицу заказов
func mkorders(c *gin.Context) {
	//читаем состояние сети
	//если сеть в процессе расчета и нет сигнала остановки, то сообщаем информацию и выходим
	// a channel to tell it to stop

	stop := c.DefaultQuery("stop", "none")
	start := c.DefaultQuery("start", "none")
	//store := c.DefaultQuery("store", "")
	//goods := c.DefaultQuery("goods", "")
	//c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "start= " + start + " stop=" + stop})
	if stop != "none" && stop != "" {
		v, err := strconv.Atoi(stop)
		if err == nil {
			x := Fop.Val()
			if int64(v) == x && x > 0 {
				Fop.Set(-1 * x)
				c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "seting Fop = " + strconv.FormatInt(Fop.Val(), 10)})
			} else {
				c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "bad stop signal. lookin for " + strconv.FormatInt(Fop.Val(), 10)})
			}
		} else {
			c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": " Atoi err = " + err.Error()})
		}
		return
	}
	if Fop.Val() != 0 {
		//находимся в состоянии расчета. выводим это
		c.JSON(http.StatusLocked, gin.H{"status": http.StatusLocked, "start": time.Unix(0, Fop.Val()).Format("2 Jan 2006 15:04:05"), "message": "в процессе составления заказа " + strconv.FormatInt(Fop.Val(), 10)})
		return
	}
	if start == "ok" {
		go makeOrders()
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "запущен расчет " + time.Now().Format("2 Jan 2006 15:04:05")})
		return
	}
	slog := models.GetLastStateNetwork(3, "makeOrders")
	status := make(map[string]string, 3)
	status["err"] = "no"
	for k, v := range slog {
		switch v[:3] {
		case "end":
			status["end"] = time.Unix(0, int64(k)).Format("2 Jan 2006 15:04:05") + v[4:]
		case "beg":
			status["beg"] = time.Unix(0, int64(k)).Format("2 Jan 2006 15:04:05") + v[4:]
		default:
			status["err"] = time.Unix(0, int64(k)).Format("2 Jan 2006 15:04:05") + v[4:]
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "beg": status["beg"], "err": status["err"], "end": status["end"]})
}

//getZakaz выгруит заказы в xml
func getZakaz(c *gin.Context) {

	period := c.DefaultQuery("date", "last")
	type OrdersXML struct {
		XMLName xml.Name          `xml:"data"`
		Orders  []models.OrderXML `xml:"orders"`
	}
	z, err := models.GetZakazXML(period)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": err.Error()})
		return
	}

	//c.XML(http.StatusOK, gin.H{"orders": z})
	c.XML(http.StatusOK, OrdersXML{Orders: z})
}

func setABC(c *gin.Context) {
	//goodsuid := c.Param("goods")
	var errmessage string
	storeuid := c.Param("store")
	if storeuid == "" {
		errmessage = "не задан uid магазина "
		c.JSON(http.StatusNotModified, gin.H{"status": http.StatusNotModified, "message": errmessage})
		return
	}
	period1 := c.DefaultPostForm("dfrom", time.Now().AddDate(0, -3, 0).Format("2006-01-02"))
	period2 := c.DefaultPostForm("dto", time.Now().Format("2006-01-02"))
	//прверим даты на корректность
	_, err := time.Parse("2006-01-02", period1)
	if err != nil {
		period1 = time.Now().AddDate(0, -3, 0).Format("2006-01-02")
		errmessage = "период1 задан не верно, принята дата " + period1 + ". "
	}
	_, err = time.Parse("2006-01-02", period2)
	if err != nil {
		period2 = time.Now().Format("2006-01-02")
		errmessage = errmessage + "период2 задан не верно, принята дата " + period2 + ". "
	}
	err = recalcABC(storeuid, period1, period2)
	if err != nil {
		c.JSON(http.StatusNotModified, gin.H{"status": http.StatusNotModified, "message": errmessage + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
}

//recalcABC расчет АВС классификации товара для склада uidStore и товара uidGoods в течении периода period
func recalcABC(uidStore string, period1 string, period2 string) error {
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
			log.Println(err)
			return err
		}
	}
	return nil
}

func setsalesmatrix(c *gin.Context) {
	//storeuid := c.Param("store")
	type Smatrix struct {
		Uidstore string  `json:"uidstore" binding:"required"`
		Uidgoods string  `json:"uidgoods" binding:"required"`
		Minimum  float64 `json:"minimum" binding:"required"`
		Maximum  float64 `json:"maximum" binding:"required"`
		Inuse    bool    `json:"inuse" binding:"required"`
		Step     float64 `json:"step" binding:"required"`
	}
	var sm []Smatrix
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}

	matr := make([]map[string]interface{}, 0, 256)
	for _, v := range sm {
		m := make(map[string]interface{})
		m["uidStore"] = v.Uidstore
		m["uidGoods"] = v.Uidgoods
		m["minbalance"] = v.Minimum
		m["maxbalance"] = v.Maximum
		if v.Inuse {
			m["inuse"] = 1
		} else {
			m["inuse"] = 0
		}
		m["step"] = v.Step
		matr = append(matr, m)
	}
	err := models.ReplaceMatrix(matr)
	if err != nil {
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
}

//стартовая страница
func startPage(c *gin.Context) {
	// Вызовем метод HTML из Контекста Gin для обработки шаблона
	// gin.H is a shortcut for map[string]interface{}
	hdata := make(map[string]interface{})
	hdata["Page"] = "home"
	hdata["User"] = "DM"
	hdata["Title"] = "Главная"
	conf, err := models.GetConfig()
	if err != nil {
		hdata["Error"] = err.Error()
	}

	if len(conf) == 0 {
		//переходим на страницу конфигурации
		c.Request.URL.Path = "/config"
		//api.HandleContext(c)
		//c.Redirect(http.StatusContinue, "config")
	}

	lastdblog := models.GetLastStateNetwork(5, "")
	ki := make([]int, 0, len(lastdblog))
	for k := range lastdblog {
		ki = append(ki, k)
	}
	sort.Ints(ki)
	var s string
	for _, v := range ki {
		s = s + time.Unix(0, int64(v)).Format("2 Jan 2006 15:04:05") + " " + lastdblog[v] + "<br />"
	}
	hdata["Neurostatus"] = template.HTML(s)
	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"index",
		// Передадим данные в шаблон
		hdata,
	)

}

func confPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	hdata["Page"] = "config"
	var cfg = make(models.Config)

	firmName, ok := c.GetQuery("firmName")
	if ok {
		//есть параметры, сохраняемся
		cfg["firmName"] = firmName
		qs := c.Request.URL.Query() //map[string][]string
		for k, v := range qs {
			cfg[k] = v[0]
		}
		logstr, err := cfg.Save()
		if err != nil {
			hdata["Error"] = err.Error() + " " + logstr
		} /*else {
			hdata["error"] = logstr
		}*/
	}

	cfg, err := models.GetConfig()
	if err != nil {
		cfg["Error"] = err.Error()
	}
	for k, v := range cfg {
		hdata[k] = v
	}
	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"config",
		// Передадим данные в шаблон
		hdata,
		/*
			gin.H{
				"page":         "config",
				"firmName":     conf["firmName"],
				"minday2sale":  conf["minday2sale"],
				"kfnunvisible": conf["kfnunvisible"],
				"maxsales":     conf["maxsales"],
				"add": func(a int, b int) string {
					return " active"
				},
				//"payload": articles,
			},
		*/
	)
}

func tablesPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	hdata["Page"] = "tables"
	//var tables = make(map[string]interface{})

	tabName := c.DefaultQuery("tab", "contracts")
	pgq := c.DefaultQuery("pg", "0")
	gateq := c.DefaultQuery("gate", "50")
	pg, ok := strconv.Atoi(pgq)
	if ok != nil {
		pg = 0
	}
	gate, ok := strconv.Atoi(gateq)
	if ok != nil {
		gate = 50
	}
	hdata["Gate"] = gate //strconv.FormatInt(int64(gate), 10)
	hdata["Pg"] = pg     //strconv.FormatInt(int64(pg), 10)
	hdata["Tabname"] = tabName
	hdata["Rutabname"] = RuName(tabName)
	recs, s, data, err := models.GetTable(tabName, pg, gate, "")
	if err != nil {
		hdata["Error"] = err.Error()
	}
	hdata["Error"] = s
	//paginator
	//определим текущий блок страниц
	if int(recs/gate)+1 > 10 {
		z := make([]int, 10)
		for i, cp := 0, int(pg/10)*10; i < 10; i++ {
			z[i] = cp + i
		}
		hdata["Pagination"] = z
		hdata["Nextpages"] = int(pg/10)*10 + 10
		hdata["Prevpages"] = int(pg/10)*10 - 10
		if int(pg/10)*10-10 < 0 {
			hdata["Prevpages"] = 0
		}
	} else {
		hdata["Nextpages"] = 0
		hdata["Prevpages"] = 0
		z := make([]int, int(recs/gate)+1)
		for i := range z {
			z[i] = i
		}
		hdata["Pagination"] = z
	}
	//строим подобную строку для таблиц и графиков
	/*
	   ['Employee Name', 'Salary'],
	   ['Mike', {v:22500, f:'22,500'}], // Format as "22,500".
	   ['Bob', 35000],
	   ['Fritz', 18500]
	*/
	datatab := "["
	dataval := data[0]
	comma := ""
	for i := 0; i < len(dataval); i++ {
		datatab = datatab + comma + "'" + dataval[i].(string) + "'"
		comma = ","
	}
	datatab = datatab + "]"
	for r := 1; r < len(data); r++ {
		datatab = datatab + ",["
		dataval = data[r]
		comma = ""
		for i := 0; i < len(dataval); i++ {
			switch dataval[i].(type) {
			case nil:
				datatab = datatab + comma + "'null'"
			case int:
				datatab = datatab + comma + strconv.FormatInt(int64(dataval[i].(int)), 10)
			case int64:
				datatab = datatab + comma + strconv.FormatInt(dataval[i].(int64), 10)
			case float64:
				datatab = datatab + comma + strconv.FormatFloat(dataval[i].(float64), 'f', 2, 64)
			case bool:
				datatab = datatab + comma + strconv.FormatBool(dataval[i].(bool))
			case string:
				datatab = datatab + comma + "'" + dataval[i].(string) + "'"
			default:
				datatab = datatab + comma + "'?'"
			}
			comma = ","
		}
		datatab = datatab + "]"
	}
	hdata["Datatab"] = template.JS(datatab)
	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		"tables",
		// Передадим данные в шаблон
		hdata,
	)
}

func main() {
	port := flag.Int("port", 3000, "Номер порта")
	portstr := ":" + strconv.Itoa(*port)
	dbpath := flag.String("db", "C:\\usr\\rszak.db", "путь к базе")
	flag.Parse()
	err := models.InitDB(*dbpath)
	if err != nil {
		panic("не смог открыть базу")
	}
	defer models.DB.Close()

	//fs := http.FileServer(http.Dir("./assets/"))
	//http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	router := gin.Default()
	router.Static("/assets", "./assets")
	//router.StaticFS("/more_static", http.Dir("my_file_system"))
	//router.StaticFile("/favicon.ico", "./resources/favicon.ico")
	//new template engine
	router.HTMLRender = gintemplate.New(gintemplate.TemplateConfig{
		Root:      "tpl",
		Extension: ".html",
		Master:    "base",
		Funcs: template.FuncMap{
			"setActive": func(a, b string) string {
				if a == b {
					return " active"
				}
				return ""
			},
			"isError": func(e string) bool {
				return len(e) > 0
			},
			"copy": func() string {
				return time.Now().Format("2006")
			},
		},
		DisableCache: true,
	})

	//router.LoadHTMLGlob("tpl/*")
	//router.LoadHTMLFiles("templates/template1.html", "templates/template2.html")
	router.GET("/", startPage)
	router.GET("/config", confPage)
	router.GET("/tables", tablesPage)
	api := router.Group("/api/")
	{
		api.GET("calc/", calculate)
		api.GET("stocks/", fetchAllStocks)
		api.GET("goods/:id", fetchSingleGoods)
		api.PUT("goods/:id", updateGoods)
		api.GET("neuro/:store/:goods", getNeuroData)
		api.GET("predict/:store/:goods", getPredict)
		api.POST("setsales/", setSales)
		api.GET("makeorders/", mkorders)
		api.POST("recalcabc/:store", setABC)
		api.GET("getOrders/", getZakaz)
		api.POST("setsalesmatrix/:store", setsalesmatrix)
		//api.DELETE("goods/:id", DeleteProduct)
	}
	router.Run(portstr)

}
