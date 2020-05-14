package main

import (
	"encoding/xml"
	"flag"

	"net/http"
	"sort"

	"strconv"
	"sync"
	"time"

	"1c8_zak/models"

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
	type Goods struct {
		Uidgoods   string `json:"uidgoods" binding:"required"`
		GroupGoods string `json:"group" binding:"required"`
		Name       string `json:"name" binding:"required"`
		Art        string `json:"art" binding:"required"`
	}
	var sm []Goods
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}
	/*
		CREATE TABLE goods (uid text PRIMARY KEY, groupname text, name text NOT NULL, art text)
	*/
	matr := make([]map[string]interface{}, 0, 256)
	for _, v := range sm {
		m := make(map[string]interface{})
		m["uid"] = v.Uidgoods
		m["groupname"] = v.GroupGoods
		m["name"] = v.Name
		m["art"] = v.Art
		matr = append(matr, m)
	}
	w := make(map[string]string)
	models.DbLog("beg. начало обновления товаров в базу "+time.Now().Format("2006-01-02T15:04:05"), "updateGoods", time.Now().UTC().UnixNano())
	err := models.InsertTableData("goods", matr, w)
	if err != nil {
		models.DbLog("err. ошибка обновления товаров в базу "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "updateGoods", time.Now().UTC().UnixNano())
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}
	models.DbLog("end. конец обновления товаров в базу "+time.Now().Format("2006-01-02T15:04:05"), "updateGoods", time.Now().UTC().UnixNano())
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
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
		Prevdays   int     `json:"prevd" binding:"required"`
		Zerodays   int     `json:"zerod" binding:"required"`
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
		if v.Period == "" {
			m["period"] = time.Now().Format("2006-01-02T15:04:05")
		} else {
			lper, err := time.Parse("2006-01-02T15:04:05", v.Period)
			if err != nil {
				//формат даты другой
				lper, err = time.Parse("2006-01-02", v.Period)
				if err != nil {
					c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": "формат даты должен быть 2006-01-02T15:02:05" + err.Error()})
					return
				}
			}
			m["period"] = lper.Format("2006-01-02T15:04:05")
		}
		if v.Tipmov == "" {
			v.Tipmov = "S"
		}
		m["tipmov"] = v.Tipmov
		m["groupGoods"] = v.GroupGoods
		m["cnt"] = v.Cnt
		m["summa"] = v.Summa
		m["margin"] = v.Margin
		m["balance"] = v.Balance
		if v.Prevdays == 0 {
			v.Prevdays = 1
		}
		m["prevdays"] = v.Prevdays
		m["zerodays"] = v.Zerodays
		matr = append(matr, m)
	}
	//для обновления записей предварительно удалим те, у которых склад, номенклатура, период и тип движения совпадают
	//потому что у нас в goodsmov итоговые движения за день!
	w := make(map[string]string)
	w["uidStore"] = "="
	w["uidGoods"] = "="
	w["period"] = "="
	w["tipmov"] = "="
	models.DbLog("beg. начало записи движений в базу "+time.Now().Format("2006-01-02T15:04:05"), "setSales", time.Now().UTC().UnixNano())
	err = models.InsRepSales(matr, w)
	if err != nil {
		models.DbLog("err. ошибка записи движений в базу "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "setSales", time.Now().UTC().UnixNano())
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}
	models.DbLog("end. конец записи движений в базу "+time.Now().Format("2006-01-02T15:04:05"), "setSales", time.Now().UTC().UnixNano())
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})

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
		go apiMakeOrders()
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
	models.DbLog("beg. выдача заказов за "+period+" "+time.Now().Format("2006-01-02T15:04:05"), "getZakaz", time.Now().UTC().UnixNano())
	z, err := models.GetZakazXML(period)
	if err != nil {
		models.DbLog("err. ошибка выдачи заказов "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "getZakaz", time.Now().UTC().UnixNano())
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
	models.DbLog("beg. начало ABC анализа "+time.Now().Format("2006-01-02T15:04:05"), "setABC", time.Now().UTC().UnixNano())
	err = apiRecalcABC(storeuid, period1, period2)
	if err != nil {
		models.DbLog("err. ошибка ABC анализа "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "setABC", time.Now().UTC().UnixNano())
		c.JSON(http.StatusNotModified, gin.H{"status": http.StatusNotModified, "message": errmessage + err.Error()})
		return
	}
	models.DbLog("end. конец ABC анализа "+time.Now().Format("2006-01-02T15:04:05"), "setABC", time.Now().UTC().UnixNano())
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
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
	w := make(map[string]string)
	//w["uidStore"] = "="
	//w["uidGoods"] = "="
	models.DbLog("beg. начало обновления матрицы товаров "+time.Now().Format("2006-01-02T15:04:05"), "setsalesmatrix", time.Now().UTC().UnixNano())
	err := models.ReplaceMatrix(matr, w)
	if err != nil {
		models.DbLog("err. ошибка обновления матрицы товаров "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "setsalesmatrix", time.Now().UTC().UnixNano())
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}
	models.DbLog("end. конец обновления матрицы товаров "+time.Now().Format("2006-01-02T15:04:05"), "setsalesmatrix", time.Now().UTC().UnixNano())
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
}

func setstores(c *gin.Context) {

	type Stores struct {
		Uidstore string `json:"uidstore" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Tip      int    `json:"tip" binding:"required"`
		Calendar string `json:"calendar" binding:"-"`
	}
	var sm []Stores
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}

	matr := make([]map[string]interface{}, 0, 256)
	for _, v := range sm {
		m := make(map[string]interface{})
		m["uid"] = v.Uidstore
		m["name"] = v.Name
		m["tip"] = v.Tip
		m["calendar"] = v.Calendar
		matr = append(matr, m)
	}
	//по ключевому полю движок sql сам обновит запись, поэтому w пустой мап
	//w["uid"]="="
	//но можно было бы сделать так
	w := make(map[string]string)
	models.DbLog("beg. начало обновления магазинов "+time.Now().Format("2006-01-02T15:04:05"), "setstores", time.Now().UTC().UnixNano())
	err := models.InsertTableData("stores", matr, w)
	if err != nil {
		models.DbLog("err. ошибка обновления магазинов "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "setstores", time.Now().UTC().UnixNano())
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}
	models.DbLog("end. конец обновления магазинов "+time.Now().Format("2006-01-02T15:04:05"), "setstores", time.Now().UTC().UnixNano())
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

//стартовая страница
func helpPage(c *gin.Context) {
	// Вызовем метод HTML из Контекста Gin для обработки шаблона
	// gin.H is a shortcut for map[string]interface{}
	hdata := make(map[string]interface{})
	hdata["Page"] = "help"
	hdata["User"] = "DM"
	hdata["Title"] = "Помощь"
	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"help",
		// Передадим данные в шаблон
		hdata,
	)

}

func main() {
	port := flag.Int("port", 3000, "Номер порта")
	portstr := ":" + strconv.Itoa(*port)
	dbpath := flag.String("db", "D:\\rszak.db", "путь к базе")
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
	router.GET("/help", helpPage)
	api := router.Group("/api/")
	{
		api.GET("calc/", calculate)
		api.GET("stocks/", fetchAllStocks)
		api.GET("goods/:id", fetchSingleGoods)
		api.PUT("goods/:id", updateGoods)
		api.GET("neuro/:store/:goods", getNeuroData)
		api.GET("predict/:store/:goods", getPredict)
		api.POST("setsales/", setSales)
		api.POST("makeorders/", mkorders)
		api.POST("recalcabc/:store", setABC)
		api.GET("getorders/", getZakaz)
		api.POST("setsalesmatrix/:store", setsalesmatrix)
		api.POST("setstores/", setstores)
		//api.DELETE("goods/:id", DeleteProduct)
	}
	router.Run(portstr)

}
