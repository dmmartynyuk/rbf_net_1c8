package main

import (
	"encoding/xml"
	"flag"
	"strings"

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

//Version версия программы
const Version = "0.4.15"

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
	//c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "stop= " + stop + " store=" + store + " goods=" + goods})
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

//fetchAllStocks выдает список всех магазинов
func fetchAllStocks(c *gin.Context) {
	uid := c.DefaultQuery("uid", "")
	name := c.DefaultQuery("name", "")
	spg := c.DefaultQuery("pageIndex", "1")
	pg, err := strconv.Atoi(spg)
	if err != nil {
		pg = 1
	}
	sgate := c.DefaultQuery("pageSize", "25")
	gate, err := strconv.Atoi(sgate)
	if err != nil {
		gate = 25
	}
	cond := ""
	if len(uid) > 0 {
		cond = "uid='" + uid + "'"
	}
	if len(name) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "name='" + name + "'"
	}
	rows, _, data, err := models.GetTable("stores", pg-1, gate, cond)
	if err != nil && rows > 0 {
		c.JSON(http.StatusNotFound, gin.H{"data": "[]", "status": http.StatusNotFound, "message": err.Error()})
		return
	}
	type Item struct {
		UID  string `json:"uid" binding:"required"`
		Name string `json:"name" binding:"required"`
		Tip  int64  `json:"tip" binding:"required"`
	}

	Items := make([]Item, 0, len(data))
	//нулевая строка содержит имена полей, пропускаем
	for r := 1; r < len(data); r++ {
		i := Item{}
		v := data[r]
		i.UID = (v[0]).(string)
		i.Name = (v[1]).(string)
		switch v[2].(type) {
		case int64:
			i.Tip = (v[2]).(int64)
		case int32:
			i.Tip = (int64)(v[2].(int32))
		case int:
			i.Tip = (int64)(v[2].(int))
		}

		Items = append(Items, i)
	}

	c.JSON(http.StatusOK, gin.H{"data": Items, "itemsCount": rows})
}

//updateStocks обновление магазинов
func updateStocks(c *gin.Context) {
	uid := c.PostForm("uid")
	name := c.PostForm("name")
	stip := c.PostForm("tip")
	tipstores, err := strconv.Atoi(stip)
	if err != nil {
		tipstores = -100
	}
	if len(uid) == 0 || (len(name) == 0 && tipstores == -100) {
		c.JSON(http.StatusNotFound, gin.H{"data": "[]"})
		return
	}
	matr := make(map[string]interface{})
	cond := make(map[string]string)
	cond["uid"] = uid
	if len(name) > 0 {
		matr["name"] = name
	}
	if tipstores != -100 {
		matr["tip"] = int64(tipstores)
	}
	m := make([]map[string]interface{}, 1)
	m[0] = matr
	err = models.UpdateTableData("stores", m, cond)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": "[]", "status": http.StatusBadRequest, "message": err.Error()})
		return
	}
	type Item struct {
		UID  string `json:"uid" binding:"required"`
		Name string `json:"name" binding:"required"`
		Tip  int64  `json:"tip" binding:"required"`
	}
	var i Item
	i.UID = uid
	i.Name = name
	i.Tip = int64(tipstores)
	c.JSON(http.StatusOK, i)
}

//fetchAllContracts выводит список контрактов
func fetchAllContracts(c *gin.Context) {
	type Item struct {
		ROWID        int64  `json:"rowid" binding:"required"`
		Provider     string `json:"provider" binding:"required"`
		Recipient    string `json:"recipient" binding:"required"`
		Providername string `json:"providername" binding:"required"`
		Recname      string `json:"recname" binding:"required"`
		Chedord      string `json:"chedord" binding:"required"`
		Delivdays    int64  `json:"delivdays" binding:"required"`
	}
	rowid := c.DefaultQuery("rowid", "")
	_, err := strconv.Atoi(rowid)
	if err != nil {
		rowid = ""
	}

	recname := c.DefaultQuery("recname", "")
	providername := c.DefaultQuery("providername", "")
	chedord := c.DefaultQuery("chedord", "")
	delivdays := c.DefaultQuery("delivdays", "")
	_, err = strconv.Atoi(delivdays)
	if err != nil {
		delivdays = ""
	}

	spg := c.DefaultQuery("pageIndex", "1")
	pg, err := strconv.Atoi(spg)
	if err != nil {
		pg = 1
	}
	sgate := c.DefaultQuery("pageSize", "25")
	gate, err := strconv.Atoi(sgate)
	if err != nil {
		gate = 25
	}
	cond := ""
	if len(rowid) > 0 {
		cond = "c.rowid=" + rowid
	}
	if len(recname) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "s.name='" + recname + "'"
	}
	if len(providername) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "c.providername='" + models.Escape(providername) + "'"
	}
	if len(chedord) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "c.chedord='" + models.Escape(chedord) + "'"
	}
	if len(delivdays) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "c.delivdays=" + delivdays
	}
	rows, _, data, err := models.GetTable("contracts", pg-1, gate, cond)
	if err != nil && rows > 0 {
		c.JSON(http.StatusNotFound, gin.H{"data": "[]", "status": http.StatusNotFound, "message": err.Error()})
		return
	}

	Items := make([]Item, 0, len(data))
	//нулевая строка содержит имена полей, пропускаем
	for r := 1; r < len(data); r++ {
		i := Item{}
		v := data[r]
		i.ROWID = (v[0]).(int64)
		i.Provider = (v[1]).(string)
		i.Recipient = (v[2]).(string)
		i.Providername = (v[3]).(string)
		i.Recname = (v[4]).(string)
		i.Chedord = (v[5]).(string)
		i.Delivdays = (v[6]).(int64)
		Items = append(Items, i)
	}

	c.JSON(http.StatusOK, gin.H{"data": Items, "itemsCount": rows})
}

//updateContracts обновление контрактов
func updateContracts(c *gin.Context) {
	type Item struct {
		ROWID        int64  `json:"rowid" binding:"required"`
		Provider     string `json:"provider" binding:"required"`
		Recipient    string `json:"recipient" binding:"required"`
		Providername string `json:"providername" binding:"required"`
		Recname      string `json:"recname" binding:"required"`
		Chedord      string `json:"chedord" binding:"required"`
		Delivdays    int64  `json:"delivdays" binding:"required"`
	}
	matr := make(map[string]interface{})
	cond := make(map[string]string)

	rowid := c.PostForm("rowid")
	rid, err := strconv.Atoi(rowid)
	if err != nil {
		rowid = ""
	}
	recname := c.PostForm("recname")
	providername := c.PostForm("providername")
	provider := c.PostForm("provider")
	recipient := c.PostForm("recipient")
	chedord := c.PostForm("chedord")
	delivdays := c.PostForm("delivdays")
	dl, err := strconv.Atoi(delivdays)
	if err != nil {
		delivdays = ""
		dl = 0
	}
	if len(rowid) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"data": "[]"})
		return
	}
	cond["rowid"] = rowid
	if len(chedord) > 0 {
		matr["chedord"] = chedord
	}
	if len(delivdays) > 0 {
		matr["delivdays"] = int64(dl)
	}
	if len(providername) > 0 && providername != "null" {
		matr["providername"] = providername
	}
	m := make([]map[string]interface{}, 1)
	m[0] = matr
	err = models.UpdateTableData("contracts", m, cond)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": "[]", "status": http.StatusBadRequest, "message": err.Error()})
		return
	}

	var i Item
	i.ROWID = int64(rid)
	i.Provider = provider
	i.Recipient = recipient
	i.Providername = providername
	i.Recname = recname
	i.Chedord = chedord
	i.Delivdays = int64(dl)
	c.JSON(http.StatusOK, i)
}

//updateContractGoods обновляет номенклатуру поставщиков
func updateContractGoods(c *gin.Context) {
	type Goods struct {
		Uidprovider string `json:"uidprovider" binding:"required"`
		Uidgoods    string `json:"uidgoods" binding:"required"`
		ProviderArt string `json:"providerart" binding:"required"`
	}
	var sm []Goods
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}
	/*
		CREATE TABLE "contractgoods" ("uidprovider"	TEXT,"uidgoods"	TEXT,"providerArt"	TEXT)
	*/
	matr := make([]map[string]interface{}, 0, len(sm))
	for _, v := range sm {
		m := make(map[string]interface{})
		m["uidprovider"] = v.Uidprovider
		m["uidgoods"] = v.Uidgoods
		m["providerart"] = v.ProviderArt
		matr = append(matr, m)
	}
	w := make(map[string]string)
	models.DbLog("beg. начало обновления товаров в базу "+time.Now().Format("2006-01-02T15:04:05"), "updateContractGoods", time.Now().UTC().UnixNano())
	err := models.InsertTableData("contractgoods", matr, w)
	if err != nil {
		models.DbLog("err. ошибка обновления товаров в базу "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "updateContractGoods", time.Now().UTC().UnixNano())
		c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		return
	}
	models.DbLog("end. конец обновления товаров в базу "+time.Now().Format("2006-01-02T15:04:05"), "updateContractGoods", time.Now().UTC().UnixNano())
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
}

//fetchAllSalesmatrix выводит матрицу
func fetchAllSalesmatrix(c *gin.Context) {
	type Item struct {
		ROWID      int64   `json:"rowid" binding:"required"`
		UIDStore   string  `json:"uidstore" binding:"required"`
		StoreName  string  `json:"storename" binding:"required"`
		UIDGoods   string  `json:"uidgoods" binding:"required"`
		GoodsGroup string  `json:"goodsgroup" binding:"required"`
		GoodsName  string  `json:"goodsname" binding:"required"`
		Art        string  `json:"art" binding:"required"`
		Minbalance float64 `json:"minbalance" binding:"required"`
		Maxbalance float64 `json:"maxbalance" binding:"required"`
		Inuse      int64   `json:"inuse" binding:"required"`
		Abc        string  `json:"abc" binding:"required"`
	}
	//sortfield:= c.DefaultQuery("sortfield", "")
	//sortorder:= c.DefaultQuery("sortorder", "") //desc  asc
	spg := c.DefaultQuery("pageIndex", "1")
	pg, err := strconv.Atoi(spg)
	if err != nil {
		pg = 1
	}
	sgate := c.DefaultQuery("pageSize", "25")
	gate, err := strconv.Atoi(sgate)
	if err != nil {
		gate = 25
	}

	rowid := c.DefaultQuery("rowid", "")
	_, err = strconv.Atoi(rowid)
	if err != nil {
		rowid = ""
	}

	storename := c.DefaultQuery("storename", "")
	goodsname := c.DefaultQuery("goodsname", "")
	goodsgroup := c.DefaultQuery("goodsgroup", "")
	art := c.DefaultQuery("art", "")
	abc := c.DefaultQuery("abc", "")
	minbalance := c.DefaultQuery("minbalance", "")
	_, err = strconv.Atoi(minbalance)
	if err != nil {
		minbalance = ""
	}
	maxbalance := c.DefaultQuery("maxbalance", "")
	_, err = strconv.Atoi(maxbalance)
	if err != nil {
		maxbalance = ""
	}
	inuse := c.DefaultQuery("inuse", "")
	_, err = strconv.Atoi(inuse)
	if err != nil {
		inuse = ""
	}
	if inuse == "-1" {
		inuse = ""
	}

	cond := ""
	if len(rowid) > 0 {
		cond = "s.rowid=" + rowid
	}
	if len(storename) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "st.name='" + models.Escape(storename) + "'"
	}
	if len(goodsname) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "g.name='" + models.Escape(goodsname) + "'"
	}
	if len(goodsgroup) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "g.groupname='" + models.Escape(goodsgroup) + "'"
	}
	if len(art) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "g.Art='" + models.Escape(art) + "'"
	}
	if len(abc) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "s.abc='" + models.Escape(abc) + "'"
	}
	if len(inuse) > 0 {
		if len(cond) > 0 {
			cond = cond + " and "
		}
		cond = cond + "s.inuse=" + inuse
	}
	rows, _, data, err := models.GetTable("salesmatrix", pg-1, gate, cond)
	if err != nil && rows > 0 {
		c.JSON(http.StatusNotFound, gin.H{"data": "[]", "status": http.StatusNotFound, "message": err.Error()})
		return
	}

	Items := make([]Item, 0, len(data))
	//нулевая строка содержит имена полей, пропускаем
	//select s.ROWID,s.uidStore,st.name as storename,s.uidGoods as uidТовара,g.name as goodsname, g.Art as art,s.minbalance as minbalance,s.maxbalance as maxbalance,s.inuse as inuse,s.abc as abc from salesmatrix as s left join stores as st on s.uidStore=st.uid left join goods as g on s.uidGoods=g.uid" + where + " order by st.name, g.groupname, g.art" + limit + ";"
	for r := 1; r < len(data); r++ {
		i := Item{}
		v := data[r]
		i.ROWID = (v[0]).(int64)
		i.UIDStore = (v[1]).(string)
		i.StoreName = (v[2]).(string)
		i.UIDGoods = (v[3]).(string)
		i.GoodsGroup = (v[4]).(string)
		i.GoodsName = (v[5]).(string)
		i.Art = (v[6]).(string)
		i.Minbalance = (v[7]).(float64)
		i.Maxbalance = (v[8]).(float64)
		i.Inuse = (v[9]).(int64)
		i.Abc = (v[10]).(string)
		Items = append(Items, i)
	}

	c.JSON(http.StatusOK, gin.H{"data": Items, "itemsCount": rows})
}

//updateSalesmatrix обновление матрицы
func updateSalesmatrix(c *gin.Context) {
	type Item struct {
		ROWID      int64   `json:"rowid" binding:"required"`
		UIDStore   string  `json:"uidstore" binding:"required"`
		StoreName  string  `json:"storename" binding:"required"`
		UIDGoods   string  `json:"uidgoods" binding:"required"`
		GoodsGroup string  `json:"goodsgroup" binding:"required"`
		GoodsName  string  `json:"goodsname" binding:"required"`
		Art        string  `json:"art" binding:"required"`
		Minbalance float64 `json:"minbalance" binding:"required"`
		Maxbalance float64 `json:"maxbalance" binding:"required"`
		Inuse      int64   `json:"inuse" binding:"required"`
		Abc        string  `json:"abc" binding:"required"`
	}
	matr := make(map[string]interface{})
	cond := make(map[string]string)

	rowid := c.PostForm("rowid")
	rid, err := strconv.Atoi(rowid)
	if err != nil {
		rowid = ""
	}
	art := c.PostForm("art")
	uidstore := c.PostForm("uidstore")
	storename := c.PostForm("storename")
	goodsgroup := c.PostForm("goodsgroup")
	uidgoods := c.PostForm("uidgoods")
	goodsname := c.PostForm("goodsname")
	abc := c.PostForm("abc")
	minbalance := c.PostForm("minbalance")
	minb, err := strconv.ParseFloat(minbalance, 64)
	if err != nil {
		minbalance = ""

	}
	maxbalance := c.PostForm("maxbalance")
	maxb, err := strconv.ParseFloat(maxbalance, 64)
	if err != nil {
		maxbalance = ""
	}
	inuse, err := strconv.Atoi(c.PostForm("inuse"))
	if err != nil {
		inuse = -1
	}
	if len(rowid) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"data": "[]"})
		return
	}
	cond["rowid"] = rowid
	if len(maxbalance) > 0 {
		matr["maxbalance"] = float64(maxb)
	}
	if len(minbalance) > 0 {
		matr["minbalance"] = float64(minb)
	}
	if len(uidstore) > 0 && uidstore != "null" {
		matr["uidstore"] = uidstore
	}
	if len(uidgoods) > 0 && uidgoods != "null" {
		matr["uidgoods"] = uidgoods
	}
	if len(abc) > 0 {
		matr["abc"] = abc
	}
	if inuse != -1 {
		matr["inuse"] = int64(inuse)
	}
	m := make([]map[string]interface{}, 1)
	m[0] = matr
	err = models.UpdateTableData("salesmatrix", m, cond)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": "[]", "status": http.StatusBadRequest, "message": err.Error()})
		return
	}

	var i Item
	i.ROWID = int64(rid)
	i.UIDStore = uidstore
	i.StoreName = storename
	i.UIDGoods = uidgoods
	i.GoodsGroup = goodsgroup
	i.GoodsName = goodsname
	i.Art = art
	i.Minbalance = minb
	i.Maxbalance = maxb
	i.Inuse = int64(inuse)
	i.Abc = abc
	c.JSON(http.StatusOK, i)
}

//updateGoods обновление Номенклатуры
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
	goodsuid := c.DefaultQuery("uid", "")
	q := c.DefaultQuery("q", "")

	if len(goodsuid) > 0 {
		gd, err := models.GetGood(goodsuid)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
			return
		}
		gds := make([]models.Goods, 1)
		gds[0] = *gd
		c.JSON(http.StatusOK, gin.H{"data": gds, "itemsCount": 1})
		//c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "uid": gd.KeyGoods, "art": gd.Art, "name": gd.Name, "group": gd.Grp})
		return
	}

	if len(q) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "нет запроса"})
		return

	}
	gds, err := models.GetGoods(q)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gds, "itemsCount": len(gds)})
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

//setSales пишет продажи в базу
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
			lper, err := time.Parse("2006-01-02", v.Period)
			if err != nil {
				//формат даты другой
				lper, err = time.Parse("2006-01-02T15:04:05", v.Period)
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
	store := models.Escape(c.DefaultQuery("store", ""))
	goods := models.Escape(c.DefaultQuery("goods", ""))
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
		go apiMakeOrders(store, goods)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "запущен расчет " + time.Now().Format("2 Jan 2006 15:04:05") + store + goods})
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

func setbalance(c *gin.Context) {
	storeuid := c.Param("store")
	type Sbalance struct {
		Uidstore   string  `json:"uidstore" binding:"required"`
		Uidgoods   string  `json:"uidgoods" binding:"required"`
		Groupgoods string  `json:"groupgoods"`
		Balance    float64 `json:"balance" binding:"required"`
	}
	var sm []Sbalance
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}
	matrbalance := make([]map[string]interface{}, 0, 256)
	for _, v := range sm {
		if storeuid == v.Uidstore {
			m := make(map[string]interface{})
			m["uidStore"] = v.Uidstore
			m["uidGoods"] = v.Uidgoods
			m["groupGoods"] = v.Groupgoods
			m["balance"] = v.Balance
			matrbalance = append(matrbalance, m)
		}
	}

	models.DbLog("beg. начало синхронизации баланса товаров "+time.Now().Format("2006-01-02T15:04:05"), "setbalance", time.Now().UTC().UnixNano())
	if len(matrbalance) > 0 {
		err := models.UpdateBalance(matrbalance)
		if err != nil {
			models.DbLog("err. ошибка синхронизации баланса товаров "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "setbalance", time.Now().UTC().UnixNano())
			c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
		}
	}
	models.DbLog("end. конец синхронизации баланса товаров "+time.Now().Format("2006-01-02T15:04:05"), "setbalance", time.Now().UTC().UnixNano())
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

func setordered(c *gin.Context) {
	type Ordered struct {
		Provider string  `json:"provider" binding:"required"`
		Uidstore string  `json:"uidstore" binding:"required"`
		Uidgoods string  `json:"uidgoods" binding:"required"`
		Period   string  `json:"period" binding:"required"`
		Cnt      float64 `json:"cnt" binding:"required"`
	}
	var sm []Ordered
	// in this case proper binding will be automatically selected
	if err := c.ShouldBindJSON(&sm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}

	for _, v := range sm {
		t, err := time.Parse("2006-01-02", v.Period)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05", v.Period)
			if err != nil {
				continue
			}
		}
		matr := make(map[string]interface{})
		matr["ordered"] = float64(v.Cnt)
		cond := make(map[string]string)
		cond["provider"] = v.Provider
		cond["uidstore"] = v.Uidstore
		cond["uidgoods"] = v.Uidgoods
		cond["period"] = t.Format("2006-01-02")
		m := make([]map[string]interface{}, 1)
		m[0] = matr
		err = models.UpdateTableData("oper", m, cond)
		if err != nil {
			models.DbLog("err. ошибка обновления заказов "+err.Error()+" "+time.Now().Format("2006-01-02T15:04:05"), "setordered", time.Now().UTC().UnixNano())
			c.JSON(http.StatusNotAcceptable, gin.H{"status": http.StatusNotAcceptable, "error": true, "message": err.Error()})
			return
		}
	}
	models.DbLog("end. конец обновления заказов "+time.Now().Format("2006-01-02T15:04:05"), "setordered", time.Now().UTC().UnixNano())
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "ok"})
}

//стартовая страница
func startPage(c *gin.Context) {
	// Вызовем метод HTML из Контекста Gin для обработки шаблона
	// gin.H is a shortcut for map[string]interface{}
	hdata := make(map[string]interface{})
	hdata["Page"] = "home"
	hdata["Version"] = Version
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

	lastdblog := models.GetLastStateNetwork(7, "")
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
	st, _ := models.GetMagNames(0, "")
	hdata["Stores"] = st
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
	hdata["Version"] = Version
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
	hdata["Version"] = Version
	//var tables = make(map[string]interface{})

	tabName := c.DefaultQuery("tab", "contracts")
	pgq := c.DefaultQuery("pageIndex", "1")
	gateq := c.DefaultQuery("pageSize", "25")
	pg, ok := strconv.Atoi(pgq)
	if ok != nil {
		pg = 1
	}
	gate, ok := strconv.Atoi(gateq)
	if ok != nil {
		gate = 25
	}
	hdata["PageSize"] = gate //strconv.FormatInt(int64(gate), 10)
	hdata["PageIndex"] = pg  //strconv.FormatInt(int64(pg), 10)
	hdata["Tabname"] = tabName
	hdata["Rutabname"] = RuName(tabName)
	var fields string
	//для каждой таблицы строим свои заголовки
	switch tabName {
	case "stores":
		fields = `[
            { name: "uid", title:"УИД",type: "text", editing: false,visible: false,width: 150 },
            { name: "name", title:"Наименование",type: "text", width: 250 },
            { name: "tip", title:"Тип",type: "number", width: 50 },
            { type: "control",deleteButton:false }
		]`
	case "goods":
		fields = `[
            { name: "uid", title:"УИД",type: "text", editing: false,visible: false,width: 150 },
            { name: "name", title:"Наименование",type: "text", width: 250 },
			{ name: "groupname", title:"Группа",type: "text", width: 70 },
			{ name: "art", title:"Тип",type: "text", width: 150 },
            { type: "control",deleteButton:false }
		]`
	case "contracts":
		fields = `[
            { name: "ROWID", title:"ИД",type: "text", editing: false,visible: false,width: 50 },
            { name: "provider", type: "text",editing: false,visible: false, width: 150 },
			{ name: "recipient", type: "text", editing: false,visible: false, width: 150 },
			{ name: "providername", title:"Поставщик",type: "text", width: 170 },
			{ name: "recname", title:"Получатель",type: "text", width: 170 },
			{ name: "chedord", title:"График заказов",type: "text", width: 100 },
			{ name: "delivdays", title:"Дней доставки",type: "number", width: 30 },
            { type: "control",deleteButton:false }
		]`
	case "salesmatrix":
		fields = `[
            { name: "ROWID", title:"ИД",type: "text", editing: false,visible: false,width: 50 },
            { name: "uidStore", type: "text",editing: false,visible: false, width: 100 },
			{ name: "storename", title:"Склад",type: "text", editing: false, width: 120 },
			{ name: "uidGoods", title:"uidТовара",editing:false,visible:false,type:"text",width:100 },
			{ name: "goodsgroup", title:"Группа",editing:false,visible:true,type:"text",width:60 },
			{ name: "goodsname", title:"Номенклатура",editing: false,type: "text", width: 180 },
			{ name: "art", title:"Артикул",editing: false,type: "text", width: 70 },
			{ name: "minbalance", title:"Мин. остаток",type: "number", width: 50 },
			{ name: "maxbalance", title:"Макс. остаток",type: "number", width: 50 },
			{ name: "inuse", title:"Для продажи",type: "select", items: [{ Name:"",Id:-1},{ Name:"Нет",Id:0},{Name:"Да",Id:1}], valueField:"Id",textField:"Name",width:50 },
			{ name: "abc", title:"ABC",type: "text", width: 30 },
            { type: "control",deleteButton:false }
		]`
	case "contactgoods":
		fields = `[
            { name: "ROWID", title:"ИД",type: "text", editing: false,visible: false,width: 50 },
            { name: "uidprovider", type: "text",editing: false,visible: false, width: 150 },
			{ name: "uidgoods", type: "text", editing: false,visible: false, width: 150 },
			{ name: "name", title:"Номенклатура",type: "text", width: 170 },
			{ name: "art", title:"Артикул",type: "text", width: 100 },
			{ name: "providerArt", title:"Артикул поставщика",type: "text", width: 100 },
            { type: "control",deleteButton:false }
		]`

	}

	hdata["Fields"] = template.JS(fields)
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
	   ['Employee Name', 'Salary'],  //заголовок
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
	//catalog
	//catalog, err := models.GetCatalog()
	//hdata["Catalog"] = template.JS(catalog)
	hdata["Datatab"] = template.JS(datatab)
	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		"tables",
		// Передадим данные в шаблон
		hdata,
	)
}

//salesPage страница движений товаров
func salesPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	hdata["Page"] = "sales"
	hdata["Version"] = Version
	hdata["User"] = "DM"
	hdata["Title"] = "Продажи"
	uidstore := c.DefaultQuery("uidstores", "")
	uidgoods := c.DefaultQuery("uidgoods", "")
	period := c.DefaultQuery("period", "")
	hdata["Uidstores"] = uidstore
	hdata["Uidgoods"] = uidgoods
	hdata["Period"] = period
	hdata["Uidstores_text"] = c.DefaultQuery("uidstores_text", "")
	hdata["Uidgoods_text"] = c.DefaultQuery("uidgoods_text", "")
	per1i, err := strconv.Atoi(period)
	if err != nil {
		per1i = 3
	}
	per2 := time.Now().Format("2006-01-02")
	per1d := time.Now().AddDate(0, -per1i, 0)
	per1 := time.Date(per1d.Year(), per1d.Month(), 1, 0, 0, 0, 0, time.Now().Location()).Format("2006-01-02")

	_, _, hdata["Stores"], _ = models.GetTable("stores", 0, 0, "tip>=0")

	hdata["SalesCounts"] = 0
	hdata["SalesProfit"] = 0
	hdata["SalesSumm"] = 0
	dataprof := "['дата','выручка','прибыль']"
	datatab := "['дата','продано','остаток']"
	if len(uidstore) > 0 && len(uidgoods) > 0 {
		datasel, _ := models.GetSales(uidstore, uidgoods, per1, per2)
		scnt := 0.0
		ssum := 0.0
		sprof := 0.0
		for k, v := range datasel.Balance {
			datatab = datatab + ",['" + time.Unix(int64(datasel.Udate[k])*86400, 0).Format("2006-01-02") + "'," + strconv.FormatFloat(datasel.Cnt[k], 'f', 0, 64) + "," + strconv.FormatFloat(v, 'f', 0, 64) + "]"
			dataprof = dataprof + ",['" + time.Unix(int64(datasel.Udate[k])*86400, 0).Format("2006-01-02") + "'," + strconv.FormatFloat(datasel.Summa[k], 'f', 0, 64) + "," + strconv.FormatFloat(datasel.Margin[k]*datasel.Summa[k], 'f', 2, 64) + "]"
			scnt = scnt + datasel.Cnt[k]
			sprof = sprof + datasel.Margin[k]*datasel.Summa[k]
			ssum = ssum + datasel.Summa[k]
		}
		hdata["SalesCounts"] = strconv.FormatFloat(scnt, 'f', 2, 64)
		hdata["SalesProfit"] = strconv.FormatFloat(sprof, 'f', 2, 64)
		hdata["SalesSumm"] = strconv.FormatFloat(ssum, 'f', 2, 64)
	}

	hdata["Datasale"] = template.JS(datatab)
	hdata["Dataprofit"] = template.JS(dataprof)

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"sales",
		// Передадим данные в шаблон
		hdata,
	)
}

//predictPage страница прогноза товаров
func predictPage(c *gin.Context) {
	type graph struct {
		Period      string
		Cnt         float64
		Balance     float64
		Sum         float64
		Prof        float64
		Margin      float64
		Prevdays    float64
		BalancePred float64
		CntPred     float64
		SumPred     float64
		ProfPred    float64
		Demand      float64
		Tab         string
	}

	GraphPeriods := make(map[string]graph)
	hdata := make(map[string]interface{})
	hdata["Page"] = "sales"
	hdata["User"] = "DM"
	hdata["Title"] = "Продажи"
	hdata["Version"] = Version
	uidstore := c.DefaultQuery("uidstores", "")
	uidgoods := c.DefaultQuery("uidgoods", "")
	period := c.DefaultQuery("period", "")
	hdata["Uidstores"] = uidstore
	hdata["Uidgoods"] = uidgoods
	hdata["Period"] = period
	hdata["Uidstorestext"] = c.DefaultQuery("uidstores_text", "")
	goodstext := c.DefaultQuery("uidgoods_text", "")
	hdata["Uidgoodstext"] = goodstext
	per1i, err := strconv.Atoi(period)
	if err != nil {
		per1i = 3
	}
	per2 := time.Now().Format("2006-01-02")
	per1d := time.Now().AddDate(0, -per1i, 0)
	per1date := time.Date(per1d.Year(), per1d.Month(), 1, 0, 0, 0, 0, time.Now().Location())
	per1 := per1date.Format("2006-01-02")
	//для списка фильтра
	_, _, hdata["Stores"], _ = models.GetTable("stores", 0, 0, "tip>=0")

	hdata["SalesCounts"] = 0
	hdata["SalesProfit"] = 0
	hdata["SalesSumm"] = 0
	mx := make([]string, 7, 7)
	hdata["Matrix"] = mx
	dataprof := "['дата','выручка','прибыль']"
	datatab := "['дата','продано','остаток','прогноз остатка']"
	if len(uidstore) > 0 && len(uidgoods) > 0 {
		var lastCenterbalance float64
		cond := "s.uidStore='" + models.Escape(uidstore) + "' and s.uidGoods='" + models.Escape(uidgoods) + "'"
		stores, err := models.GetMagNames(0, uidstore)
		if len(stores) > 0 && stores[0].Tip == 0 { //распределительный склад
			uidstore = "" //продажи и предикт суммируется
			cond = " st.tip>0 and s.uidgoods='" + models.Escape(uidgoods) + "'"
			_, lastCenterbalance, _ = models.GetLastBalance(stores[0].KeyStore, uidgoods)
		}
		datasel, _ := models.GetSales(uidstore, uidgoods, per1, per2, "SMRb") //для перемещений только баланс
		//добавим баланс распределительного склада, если смотрим для него статистику
		if len(datasel.Balance) > 0 && lastCenterbalance > 0 {
			datasel.Balance[len(datasel.Balance)-1] = datasel.Balance[len(datasel.Balance)-1] + lastCenterbalance
		}
		datapredict, _ := models.GetPredict(uidstore, uidgoods, per1, per2)

		//s.ROWID,s.uidStore,st.name as storename,s.uidGoods as uidGoods,g.groupname as groupname, g.name as goodsname, g.Art as art,s.minbalance as minbalance,s.maxbalance as maxbalance,s.inuse as inuse,s.abc as abc
		k, _, matrix, err := models.GetTable("salesmatrix", 0, 0, cond)
		if err != nil {
			mx[6] = err.Error()
		}
		switch {
		case k < 1:
			mx[6] = "Для склада в матрице товаров нет записи для " + goodstext
		case k == 1:
			mx[0] = strconv.FormatFloat(matrix[1][7].(float64), 'f', 2, 64)  //minbalance
			mx[1] = strconv.FormatFloat(matrix[1][8].(float64), 'f', 2, 64)  //maxbalance
			mx[2] = strconv.FormatInt(matrix[1][11].(int64), 10)             //step
			mx[3] = strconv.FormatInt(matrix[1][9].(int64), 10)              //inuse
			mx[4] = strings.ToUpper(matrix[1][10].(string))                  //abc
			mx[5] = strconv.FormatFloat(matrix[1][12].(float64), 'f', 4, 64) //demand
		case k > 1:
			//uidStore="" сумма по всем складам
			var m0, m1, m5 float64
			var m2 int64
			for i := 1; i < len(matrix); i++ {
				m0 = m0 + matrix[i][7].(float64) //minbalance
				m1 = m1 + matrix[i][8].(float64) //maxbalance
				if m2 > matrix[i][11].(int64) {
					m2 = matrix[i][11].(int64) //step
				}
				m5 = m5 + matrix[i][12].(float64) //demand
			}
			mx[0] = strconv.FormatFloat(m0, 'f', 2, 64)         //minbalance
			mx[1] = strconv.FormatFloat(m1, 'f', 2, 64)         //maxbalance
			mx[2] = strconv.FormatInt(m2, 10)                   //step
			mx[3] = strconv.FormatInt(matrix[1][9].(int64), 10) //inuse
			mx[4] = strings.ToUpper(matrix[1][10].(string))     //abc
			mx[5] = strconv.FormatFloat(m5, 'f', 4, 64)         //demand
		}
		//если нет движений гуугл ругается.  следовательно заполним начальную точку
		dataprof = dataprof + ",['" + per1date.Format("2006-01-02") + "',0,0]"
		scnt := 0.0
		ssum := 0.0
		sprof := 0.0
		for k, v := range datasel.Balance {
			gr := graph{}
			gr.Period = time.Unix(int64(datasel.Udate[k])*86400, 0).Format("2006-01-02")
			gr.Cnt = datasel.Cnt[k]
			gr.Balance = v
			gr.Margin = datasel.Margin[k]
			gr.Sum = datasel.Summa[k]
			gr.Prof = datasel.Margin[k] * datasel.Summa[k]
			gr.Prevdays = datasel.Prevdays[k]
			gr.Tab = "SM"
			GraphPeriods[gr.Period] = gr
			dataprof = dataprof + ",['" + time.Unix(int64(datasel.Udate[k])*86400, 0).Format("2006-01-02") + "'," + strconv.FormatFloat(datasel.Summa[k], 'f', 0, 64) + "," + strconv.FormatFloat(datasel.Margin[k]*datasel.Summa[k], 'f', 2, 64) + "]"
			scnt = scnt + datasel.Cnt[k]
			sprof = sprof + datasel.Margin[k]*datasel.Summa[k]
			ssum = ssum + datasel.Summa[k]
		}
		for _, v := range datapredict {
			gr := graph{}
			gr, ok := GraphPeriods[v.Period]
			if ok {
				gr.Demand = v.Demand
				gr.Tab = "SMR"
				GraphPeriods[v.Period] = gr
			} else {
				gr.Period = v.Period
				gr.Demand = v.Demand
				gr.Tab = "R"
				GraphPeriods[gr.Period] = gr
			}
		}
		if len(mx[5]) == 0 && len(datapredict) > 0 {
			mx[5] = strconv.FormatFloat(datapredict[0].Demand, 'f', 4, 64)
		}

		hdata["Matrix"] = mx
		hdata["SalesCounts"] = strconv.FormatFloat(scnt, 'f', 2, 64)
		hdata["SalesProfit"] = strconv.FormatFloat(sprof, 'f', 2, 64)
		hdata["SalesSumm"] = strconv.FormatFloat(ssum, 'f', 2, 64)

		var demand float64
		var prevdays float64    //дней с пред покупки
		var prevbalance float64 //пред остаток
		var prevmargin float64
		var prevprice float64
		datatab = datatab + ",['" + per1date.Format("2006-01-02") + "',0,0,0]" //point 0/0
		for per := per1date; per.Unix() < time.Now().Unix(); per = per.AddDate(0, 0, 1) {
			perd := per.Format("2006-01-02")
			gr, ok := GraphPeriods[perd]
			if ok {
				//данные от движений
				if strings.Contains(gr.Tab, "SM") {
					prevbalance = gr.Balance
					if gr.Prevdays > 0 { //данные от продаж
						//gr.CntPred = gr.Prevdays * demand
						if gr.Cnt > 0 && gr.Sum > 0 {
							//	gr.SumPred = gr.Sum/gr.Cnt * gr.CntPred
							prevprice = gr.Sum / gr.Cnt
						}
						//gr.SumPred = gr.Sum * gr.CntPred
						//gr.ProfPred = gr.Margin * gr.SumPred
						//gr.BalancePred = prevbalance - gr.CntPred
						if gr.Margin > 0 {
							prevmargin = gr.Margin
						}
						prevdays = 0
					}
				}
				//если есть предидущий прогноз
				if demand > 0 {
					gr.CntPred = prevdays * demand
					gr.SumPred = prevprice * gr.CntPred
					gr.ProfPred = prevmargin * gr.SumPred
					gr.BalancePred = prevbalance - gr.CntPred
				}
				if strings.Contains(gr.Tab, "R") {
					demand = gr.Demand
					//данные от предсказаний
					gr.CntPred = prevdays * demand
					gr.SumPred = prevprice * gr.CntPred
					gr.ProfPred = prevmargin * gr.SumPred
					gr.BalancePred = prevbalance - gr.CntPred
				}
				prevdays++
				datatab = datatab + ",['" + gr.Period + "'," + strconv.FormatFloat(gr.Cnt, 'f', 0, 64) + "," + strconv.FormatFloat(gr.Balance, 'f', 0, 64) + "," + strconv.FormatFloat(gr.BalancePred, 'f', 0, 64) + "]"
			}
		}
		datatab = datatab + ",['" + time.Now().AddDate(0, 0, 7).Format("2006-01-02") + "',,," + strconv.FormatFloat(prevbalance-demand*7, 'f', 0, 64) + "]"
	} else {
		datatab = datatab + ",['" + time.Now().Format("2006-01-02") + "',0,0,0]"
		dataprof = dataprof + ",['" + time.Now().Format("2006-01-02") + "',0,0]"
	}

	hdata["Datasale"] = template.JS(datatab)
	hdata["Dataprofit"] = template.JS(dataprof)

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"sales",
		// Передадим данные в шаблон
		hdata,
	)
}

//ordersPage страница заказов
func ordersPage(c *gin.Context) {
	const PAGERS = 10
	hdata := make(map[string]interface{})
	hdata["Page"] = "orders"
	hdata["Version"] = Version
	hdata["User"] = "DM"
	hdata["Title"] = "Заказы поставщикам"
	provider := c.DefaultQuery("provider", "")
	providertext := c.DefaultQuery("provider_text", "")
	store := c.DefaultQuery("store", "")
	//period := c.DefaultQuery("period", "")
	filter := c.DefaultQuery("pageFilter", "")
	numdoc := c.DefaultQuery("numdoc", "")
	pgq := c.DefaultQuery("pageIndex", "1")
	gateq := c.DefaultQuery("pageSize", "15")
	sortField := c.DefaultQuery("sortField", "period")
	sortOrder := c.DefaultQuery("sortOrder", "desc")
	pg, ok := strconv.Atoi(pgq)
	if ok != nil {
		pg = 1
	}
	gate, ok := strconv.Atoi(gateq)
	if ok != nil {
		gate = 15
	}
	hdata["PageSize"] = gate //strconv.FormatInt(int64(gate), 10)
	hdata["PageIndex"] = pg  //strconv.FormatInt(int64(pg), 10)
	hdata["SortField"] = sortField
	hdata["SortOrder"] = sortOrder
	hdata["Provider"] = provider
	hdata["Providertext"] = providertext
	hdata["Store"] = store
	hdata["PageFilter"] = filter
	hdata["Numdoc"] = numdoc
	type Hselect struct {
		UID  string
		Name string
	}
	//массив внешних поставщиков
	_, _, prov, _ := models.GetTable("contracts", 0, 0, "s.tip=0")
	sel := make([]Hselect, len(prov))
	for k := 1; k < len(prov); k++ {
		sel[k-1].UID = prov[k][1].(string)
		sel[k-1].Name = prov[k][3].(string)
	}
	hdata["Providers"] = sel
	//массив получателей
	_, _, prov, _ = models.GetTable("contracts", 0, 0, "s.tip>=1")
	sel = make([]Hselect, len(prov))
	for k := 1; k < len(prov); k++ {
		sel[k-1].UID = prov[k][2].(string)
		sel[k-1].Name = prov[k][4].(string)
	}
	hdata["Recipients"] = sel
	recs, zakazs, err := models.GetZakaz(numdoc, pg, gate, sortField, sortOrder, filter)
	if err != nil {
		hdata["Error"] = err.Error()
	}

	hdata["Zaks"] = zakazs
	hdata["Fpage"] = (pg-1)*gate + 1
	//paginator
	//определим текущий блок страниц
	if int(recs/gate) > PAGERS {
		z := make([]int, 0, PAGERS)
		//pg счет с 1, поэтому pg-1
		//fp := (pg-1)*gate + 1
		for i, cp := 0, int((pg-1)/PAGERS)*PAGERS+1; i < PAGERS && gate*(cp+i-1)+1 <= recs; i++ {
			z = append(z, 0)
			z[i] = cp + i
		}
		hdata["Pagination"] = z
		if (int((pg-1)/PAGERS)*PAGERS+PAGERS)*gate < recs {
			hdata["Nextpages"] = int((pg-1)/PAGERS)*PAGERS + PAGERS + 1
		}
		hdata["Prevpages"] = int(pg/PAGERS)*PAGERS - PAGERS + 1
		if int(pg/PAGERS)*PAGERS-PAGERS < 0 {
			hdata["Prevpages"] = 1
		}
	} else {
		//страниц мало, до 10
		hdata["Nextpages"] = 1
		hdata["Prevpages"] = 1
		z := make([]int, int(recs/gate)+1)
		for i := range z {
			z[i] = i + 1
		}
		hdata["Pagination"] = z
	}
	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"orders",
		// Передадим данные в шаблон
		hdata,
	)
}

//getOrder выгруит заказ
func getOrder(c *gin.Context) {

	numdoc := c.Param("numdoc")

	recs, zakazs, err := models.GetZakaz(numdoc, 0, 0, "", "", "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error()})
		return
	}

	//c.XML(http.StatusOK, gin.H{"orders": z})
	c.JSON(http.StatusOK, gin.H{"recs": recs, "orders": zakazs})
}

//helpPage страница справочника
func helpPage(c *gin.Context) {
	// Вызовем метод HTML из Контекста Gin для обработки шаблона
	// gin.H is a shortcut for map[string]interface{}
	hdata := make(map[string]interface{})
	hdata["Page"] = "help"
	hdata["Version"] = Version
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

func finPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	hdata["Page"] = "finance"
	hdata["Version"] = Version
	hdata["User"] = "DM"
	hdata["Title"] = "Финансовые показатели"

	uidstore := c.DefaultQuery("uidstores", "")
	period := c.DefaultQuery("period", "")
	hdata["Uidstores"] = uidstore
	hdata["Period"] = period
	hdata["Uidstorestext"] = c.DefaultQuery("uidstores_text", "")

	per, err := strconv.Atoi(period)
	if err != nil {
		per = 12
	}
	pfrom := time.Now().AddDate(0, -per-1, 0).Format("2006-01-02")
	//для списка фильтра
	_, _, hdata["Stores"], _ = models.GetTable("stores", 0, 0, "tip>0")

	hdata["SalesCounts"] = 0
	hdata["SalesProfit"] = 0
	hdata["SalesSumm"] = 0
	dataprof := "['дата','выручка','прибыль']"
	//для накопления результата
	gr := make(map[string]models.ProfitGraph)
	//datafin, err := models.GetProfitMounth("2017-01-01", time.Now().Format("2006-01-02"))
	if uidstore == "0" {
		uidstore = ""
	}
	datafin, qerr, err := RecalcProfit(uidstore, pfrom, "")
	hdata["NetErr"] = int(qerr*10000/2) / 100
	if err != nil {
		hdata["Error"] = err.Error()
	}

	//если нет ничего гуугл ругается.  следовательно заполним начальную точку
	//dataprof = dataprof + ",['" + per1date.Format("2006-01-02") + "',0,0,0]"
	//var scnt int64
	var ssum int64
	var sprof int64
	for k, datamag := range datafin {
		if uidstore == "" || k == uidstore {
			for p, v := range datamag {
				if p >= len(datamag)-per {
					prg, ok := gr[v.Period]
					if ok {
						//prg.Cnt = prg.Cnt + v.Cnt
						prg.Profit = prg.Profit + v.Profit
						prg.Proceed = prg.Proceed + v.Proceed
						gr[v.Period] = prg
					} else {
						t := models.ProfitGraph{}
						t.Period = v.Period
						t.Proceed, t.Profit = v.Proceed, v.Profit
						gr[v.Period] = t
					}
					//scnt = scnt + v.Cnt
					sprof = sprof + v.Profit
					ssum = ssum + v.Proceed
				}
			}
		}
	}
	//для сортировки по дате
	datesort := make([]string, 0, len(gr))
	for k := range gr {
		if k != "" {
			datesort = append(datesort, k)
		}
	}
	sort.Strings(datesort)
	//get result from gr
	for _, p := range datesort {
		v := gr[p]
		if v.Period != "" {
			//dataprof = dataprof + ",['" + v.Period + "'," + strconv.FormatInt(v.Proceed, 10) + "," + strconv.FormatInt(v.Profit, 10) + "," + strconv.FormatInt(v.Cnt, 10) + "]"
			dataprof = dataprof + ",['" + v.Period + "'," + strconv.FormatInt(v.Proceed, 10) + "," + strconv.FormatInt(v.Profit, 10) + "]"
		}
	}

	//hdata["SalesCounts"] = strconv.FormatInt(scnt, 10)
	hdata["SalesProfit"] = strconv.FormatInt(sprof, 10)
	hdata["SalesSumm"] = strconv.FormatInt(ssum, 10)

	hdata["Dataprofit"] = template.JS(dataprof)

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"finance",
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
			"Plus": func(a, b int) int {
				return a + b
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
	router.GET("/sales", predictPage)
	router.GET("/orders", ordersPage)
	router.GET("/finance", finPage)
	api := router.Group("/api/")
	{
		api.POST("calc/", calculate)

		api.GET("stores/", fetchAllStocks)
		api.PUT("stores/", updateStocks)
		api.POST("setstores/", setstores)
		//api.POST("stores/", InsertStocks)
		//api.DELETE("stores/", deleteStocks)

		api.GET("contracts/", fetchAllContracts)
		api.PUT("contracts/", updateContracts)
		api.POST("contractgoods/", updateContractGoods)

		api.GET("salesmatrix/", fetchAllSalesmatrix)
		api.PUT("salesmatrix/", updateSalesmatrix)
		api.POST("setsalesmatrix/:store", setsalesmatrix)
		api.POST("setbalance/:store", setbalance)

		api.GET("goods/", fetchSingleGoods)
		api.POST("goods/", updateGoods)

		api.GET("neuro/:store/:goods", getNeuroData)
		api.GET("predict/:store/:goods", getPredict)
		api.POST("setsales/", setSales)
		api.POST("setordered/", setordered)
		api.POST("makeorders/", mkorders)
		api.POST("recalcabc/:store", setABC)
		api.GET("getorders/", getZakaz)
		api.GET("getorder/:numdoc", getOrder)

		//api.DELETE("goods/:id", DeleteProduct)
	}
	router.Run(portstr)

}
