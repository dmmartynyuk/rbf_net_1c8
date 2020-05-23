package models

import (
	"database/sql"
	"encoding/xml"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB указатель на базу данных
var DB *sql.DB

//Stores магазины
type Stores map[string]string

// User stores information of a users
type User struct {
	Name  string
	Email string
	Intro string
}

//ZakazGoods строки заказа
type ZakazGoods struct {
	//Gds товар
	Gds   string
	Price float64
	Cnt   float64
}

//Zakaz заказы
type Zakaz struct {
	//Provider uid поставщика
	Provider string
	//Recipient uid получателя
	Recipient string
	//Period дата заказа
	Period string
	//Num номер заказа
	Num string
	//DelivPeriod следующая дата поставки
	DelivPeriod string
	//Items массив строк заказа
	Items []ZakazGoods
}

//ItemXML структура строк заказов
type ItemXML struct {
	XMLName xml.Name `xml:"item"`
	Gds     string   `xml:"item_id"`
	Price   float64  `xml:"price"`
	Cnt     float64  `xml:"amount"`
}

//ItemsXML контейнер для всех ItemsXML
type ItemsXML struct {
	XMLName xml.Name `xml:"items"`
	Items   []ItemXML
}

//OrderXML заказ для xml
type OrderXML struct {
	Num         string `xml:"number"`
	Period      string `xml:"order_date"`
	Provider    string `xml:"supply_warehouse_id"`
	Recipient   string `xml:"warehouse_id"`
	DelivPeriod string `xml:"delivery_date"`
	Items       ItemsXML
}

// Store Описание склада
type Store struct {
	KeyStore string
	Name     string
	Tip      string
	Calendar string
}

// Contract Описание поставок
type Contract struct {
	//Provider uid поставщика
	Provider string
	//Recipient uid получателя
	Recipient string
	//Chedord расписание заказов
	Chedord string
	//Cheddeliv расписание поставок
	Cheddeliv string
	//Delivdays количество дней от заказа до поставки
	Delivdays int
}

// Goods Описание товаров
type Goods struct {
	KeyGoods string
	Grp      string
	Name     string
	Art      string
}

// MatrixGoods Описание товаров из матрицы
type MatrixGoods struct {
	//KeyGoods uid товара
	KeyGoods string
	//MaxBalance страховой запас товара на складе
	MinBalance float64
	//MaxBalance максимальное количество товара на складе
	MaxBalance float64
	//Vitrina количество товара на витрине, который не продается
	Vitrina float64
	//Abc класс товара
	Abc string
	//Balance текущий баланс по магазину
	Balance float64
	//PredPeriod дата расчета прогноза
	PredPeriod string
	//PredDays predict days прогноз частоты покупок в днях, меньше-чаще predict days
	PredDays int
	//PredCnt прогноз количества покупок в течении PredDays
	PredCnt float64
	//PredDemand прогноз потребности ед/день
	PredDemand float64
	//Step кратность упаковки товара
	Step float64
}

//SQLiteObject.Execute("CREATE TABLE IF NOT EXISTS stores (uid text PRIMARY KEY, name text NOT NULL, tip integer)");
//SQLiteObject.Execute("CREATE TABLE IF NOT EXISTS goods (uid text PRIMARY KEY, groupname text, name text NOT NULL, art text)");
//SQLiteObject.Execute("CREATE TABLE IF NOT EXISTS goodsmov (id integer PRIMARY KEY, uidStore text NOT NULL, uidGoods text NOT NULL, groupGoods text, period text NOT NULL, cnt real, summa integer, margin real, balance real, prevdays integer, zerodays integer)");
//SQLiteObject.Execute("CREATE TABLE IF NOT EXISTS neuro (id integer PRIMARY KEY, uidStore text NOT NULL, uidGoods text NOT NULL, netdata text NOT NULL, period text, sigmaper real)");
//SQLiteObject.Execute("CREATE TABLE IF NOT EXISTS predict (id integer PRIMARY KEY, uidStore text NOT NULL, uidGoods text NOT NULL, period text NOT NULL, cnt integer)");

//Goodsmov таблица продаж
type goodsmov struct {
	keyStore string
	keyGoods string
	grp      string
	period   string
	//Udate Period в формате unix
	udate    sql.NullFloat64
	cnt      sql.NullFloat64
	summa    sql.NullFloat64
	margin   sql.NullFloat64
	balance  sql.NullFloat64
	prevdays sql.NullFloat64
	zerodays sql.NullFloat64
	tipmov   int
}

//Config таблица настроек
type Config map[string]string

// Sales Продажи для расчета
type Sales struct {
	KeyStore string
	KeyGoods string
	Grp      [3]rune
	//LastBalance последний остаток по складу на последнюю дату движения
	LastBalance float64
	//Udate Period в формате unix
	Udate []float64
	//LastPeriod последняя дата движения
	LastPeriod float64
	Cnt        []float64
	Summa      []float64
	Margin     []float64
	Balance    []float64
	Prevdays   []float64
	Zdays      []float64
}

//Neuro содержит данные строки из базы
type Neuro struct {
	KeyStore string
	KeyGoods string
	Netdata  string
	Period   string
	SigmaPer float64
}

//Predict содержит данные строки из базы
type Predict struct {
	//KeyStore склад прогноза
	KeyStore string
	//KeyGoods товар прогноза
	KeyGoods string
	//Period период составления прогноза
	Period string
	//Cnt прогнозируемое количество следующей покупки
	Cnt float64
	//Days прогнозируемый период следующей покупки
	Days int
	//Demand прогнозируемая потребность шт в день
	Demand float64
}

//Escape экранирует символы для sql
func Escape(source string) string {
	var j int = 0
	if len(source) == 0 {
		return ""
	}
	tempStr := source[:]
	desc := make([]byte, len(tempStr)*2)
	for i := 0; i < len(tempStr); i++ {
		flag := false
		var escape byte
		switch tempStr[i] {
		case '\r':
			flag = true
			escape = '\r'
			break
		case '\n':
			flag = true
			escape = '\n'
			break
		case '\\':
			flag = true
			escape = '\\'
			break
		case '\'':
			flag = true
			escape = '\''
			break
		case '"':
			flag = true
			escape = '"'
			break
		case '\032':
			flag = true
			escape = 'Z'
			break
		default:
		}
		if flag {
			desc[j] = '\\'
			desc[j+1] = escape
			j = j + 2
		} else {
			desc[j] = tempStr[i]
			j = j + 1
		}
	}
	return string(desc[0:j])
}
func escstr(s string) string {
	res := strings.ReplaceAll(s, "'", "''")
	res = strings.ReplaceAll(res, ";", "\\;")
	res = strings.ReplaceAll(res, "\"", "''")
	res = strconv.Quote(res)
	return res
}
func deescstr(s string) string {
	res := strings.ReplaceAll(s, "''", "\\'")
	res = strings.ReplaceAll(res, `\"`, `"`)
	res = strings.ReplaceAll(res, "\\;", ";")
	res = strings.ReplaceAll(res, "\\n", "")
	res = strings.ReplaceAll(res, "\\t", "")
	res = strings.ReplaceAll(res, "\\r", "")
	return res
}

//Dgraf хранит результат для построения графика в виде массива js
type Dgraf map[string]map[string]int

// Add добавляет в мап мапов данные таблицы
func (dg Dgraf) Add(x, y string, val int) {
	if y != "" && x != "" {
		row, ok := dg[x]
		if !ok {
			row = make(map[string]int)
			row[y] = val
			dg[x] = row
			//dg[x][y]=val
		}
		row[y] = val
	}
}

// Get метод Dgraf, вернет данные из мапа
func (dg Dgraf) Get(x, y string) int {
	row, ok := dg[x]
	if !ok {
		return 0
	}
	i, ok := row[y]
	if !ok {
		return 0
	}
	return i
}

// InitDB получает ссылку на DB
func InitDB(dataSourceName string) error {
	var err error
	DB, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return err
		//log.Panic(err)
	}
	if err = DB.Ping(); err != nil {
		return err
		//log.Fatal(err)
	}
	return err
}

//GetTable вернет количество строк в запросе, []map[int]interface{} где map[0] ключевое поле [0]map имена полей
func GetTable(tname string, page int, gate int, cond string) (int, string, []map[int]interface{}, error) {
	var s string
	var recs string
	var rcount int
	limit := " limit " + strconv.Itoa(gate*page) + "," + strconv.Itoa(gate)
	if gate == 0 {
		limit = ""
	}
	var where string
	if len(cond) > 0 {
		where = " where " + cond
	}
	switch tname {
	case "stores":
		s = "select uid, name, tip from stores" + where + " order by name " + limit + ";"
		recs = "select count(*) from stores " + where + ";"
	case "goods":
		s = "select uid, name,groupname, art from goods" + where + " order by groupname, art " + limit + ";"
		recs = "select count(*) from goods " + where + ";"
	case "contracts":
		//s = "select ROWID, provider,recipient,chedord,cheddeliv,delivdays from contracts" + where + limit + ";"
		s = "select c.ROWID, c.provider as provider,c.recipient as recipient, c.providername as providername, s.name as recname,c.chedord as chedord,c.delivdays as delivdays from contracts as c left join stores as s on c.recipient=s.uid" + where + limit + ";"
		recs = "select count(c.ROWID) from contracts as c left join stores as s on c.recipient=s.uid" + where + ";"
	case "contactgoods":
		s = "select c.ROWID, c.uidprovider,c.uidgoods as uid,s.name as goodsname, s.art as art, c.providerArt as providerart from contractgoods as c left join goods as s on c.uidgoods=s.uid" + where + " order by s.art " + limit + ";"
		recs = "select count(c.ROWID) from contractgoods as c left join goods as s on c.uidgoods=s.uid" + where + ";"
	case "salesmatrix":
		//s = "select s.uidStore,st.name as Склад,s.uidGoods as uidТовара,g.name as Номенклатура, g.Art as артикул,s.minbalance as МинОстаток,s.maxbalance as МаксОстаток,s.cost,s.vitrina,s.midperiod,s.demand,s.price,s.margin,s.inuse as ВПродаже,s.abc from salesmatrix as s left join stores as st on s.uidStore=st.uid left join goods as g on s.uidGoods=g.uid" + where + limit + ";"
		s = "select s.ROWID,s.uidStore,st.name as storename,s.uidGoods as uidGoods,g.groupname as groupname, g.name as goodsname, g.Art as art,s.minbalance as minbalance,s.maxbalance as maxbalance,s.inuse as inuse,s.abc as abc from salesmatrix as s left join stores as st on s.uidStore=st.uid left join goods as g on s.uidGoods=g.uid" + where + " order by st.name, g.groupname, g.art" + limit + ";"
		recs = "select count(s.ROWID) from salesmatrix as s left join stores as st on s.uidStore=st.uid left join goods as g on s.uidGoods=g.uid" + where + ";"
	}
	rows, err := DB.Query(recs)
	if err != nil {
		return 0, "", nil, err
		//log.Panic(err)
	}
	if rows.Next() {
		err := rows.Scan(&rcount)
		if err != nil {
			rows.Close()
			return 0, "", nil, err
		}
	}
	rows.Close()
	rows, err = DB.Query(s)
	if err != nil {
		return 0, s, nil, err
		//log.Panic(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return 0, s, nil, err
		//panic(err)
	}
	lencol := len(columns)
	result := make([]map[int]interface{}, 0)
	value := make(map[int]interface{})
	for i := 0; i < lencol; i++ {
		value[i] = columns[i]
	}
	result = append(result, value)
	for rows.Next() {
		row := make([]interface{}, 0, lencol)
		//инициализируем row
		for i := 0; i < lencol; i++ {
			var current interface{}
			current = struct{}{}
			row = append(row, &current)
		}
		//читаем таблицу в row
		if err := rows.Scan(row...); err != nil {
			return 0, s, nil, err
			//panic(err)
		}
		value := make(map[int]interface{})
		for i := 0; i < lencol; i++ {
			//key := columns[i]
			key := i
			//приводим к интерфейсу
			val := *(row[i]).(*interface{})
			if val == nil {
				value[key] = "null"
				continue
			}
			switch val.(type) {
			case int:
				value[key] = val.(int64)
			case int64:
				value[key] = val.(int64)
			case string:
				value[key] = deescstr(val.(string))
			case time.Time:
				value[key] = val.(time.Time).Format("2006-01-02T15:04:05")
			case []uint8:
				value[key] = string(val.([]uint8))
			case float64:
				value[key] = val.(float64)
			case bool:
				value[key] = val.(bool)
			default:
				value[key] = "?"
				//fmt.Printf("unsupport data type '%s' now\n", vType)
				// TODO remember add other data type
			}
		}
		result = append(result, value)
	}
	return rcount, s, result, err

}

//InsertTableData изменяет таблицу tabname
func InsertTableData(tabname string, matr []map[string]interface{}, keydel map[string]string) error {
	itrans := 500
	s := ""
	i := 0
	for _, m := range matr {
		flds := ""
		val := ""
		comma := ""
		and := ""
		del := ""
		for k, v := range m {
			flds = flds + comma + k
			delcomp, ok := keydel[k]
			var valstr string
			switch v.(type) {
			case float64:
				valstr = strconv.FormatFloat((v.(float64)), 'f', -1, 64)
			case int64:
				valstr = strconv.FormatInt((v.(int64)), 10)
			case int:
				valstr = strconv.FormatInt(int64(v.(int)), 10)
			case float32:
				valstr = strconv.FormatFloat(float64(v.(float64)), 'f', -1, 64)
			case time.Time:
				valstr = "'" + (v.(time.Time)).Format("2006-01-02T15:04:05") + "'"
			case bool:
				if (v.(bool)) == true {
					valstr = "true"
				} else {
					valstr = "false"
				}
			case string:
				valstr = "'" + Escape(v.(string)) + "'"
			}
			val = val + comma + valstr
			if ok {
				del = del + and + k + delcomp + valstr
				and = " and "
			}
			comma = ","
		}
		if len(del) > 0 {
			del = "DELETE FROM " + tabname + " WHERE " + del + "; "
		}
		s = s + del + "INSERT OR REPLACE INTO " + tabname + " (" + flds + ") VALUES(" + val + ");"
		i++
		if i > itrans {
			i = 0
			s = "BEGIN TRANSACTION;" + s + "COMMIT TRANSACTION;"
			_, err := DB.Exec(s)
			if err != nil {
				DB.Exec("ROLLBACK TRANSACTION;")
				return err
			}
			s = ""
		}
	}
	if s != "" {
		s = "BEGIN TRANSACTION;" + s + "COMMIT TRANSACTION;"
		_, err := DB.Exec(s)
		if err != nil {
			DB.Exec("ROLLBACK TRANSACTION;")
			return err
		}
	}
	return nil
}

//UpdateTableData изменяет таблицу tabname согласно условию w
func UpdateTableData(tabname string, matr []map[string]interface{}, w map[string]string) error {
	itrans := 500
	var and string

	where := " where "
	for k, v := range w {
		where = where + and + k + "='" + Escape(v) + "'"
		and = " and "
	}
	s := ""
	i := 0
	for _, m := range matr {
		val := ""
		comma := ""
		set := " set "
		for k, v := range m {
			switch v.(type) {
			case float64:
				val = val + comma + set + k + "=" + strconv.FormatFloat((v.(float64)), 'f', -1, 64)
			case float32:
				val = val + comma + set + k + "=" + strconv.FormatFloat(float64(v.(float32)), 'f', -1, 64)
			case int64:
				val = val + comma + set + k + "=" + strconv.FormatInt((v.(int64)), 10)
			case int:
				val = val + comma + set + k + "=" + strconv.FormatInt(int64(v.(int)), 10)
			case time.Time:
				val = val + comma + set + k + "='" + (v.(time.Time)).Format("2006-01-02T15:04:05") + "'"
			case bool:
				if (v.(bool)) == true {
					val = val + comma + set + k + "=true"
				} else {
					val = val + comma + set + k + "=false"
				}
			case string:
				val = val + comma + set + k + "='" + Escape(v.(string)) + "'"
			}
			comma = ","
			set = ""
		}
		s = s + "UPDATE " + tabname + val + where + ";"
		i++
		if i > itrans {
			i = 0
			s = "BEGIN TRANSACTION;" + s + "COMMIT TRANSACTION;"
			_, err := DB.Exec(s)
			if err != nil {
				DB.Exec("ROLLBACK TRANSACTION;")
				return err
			}
			s = ""
		}
	}
	if s != "" {
		s = "BEGIN TRANSACTION;" + s + "COMMIT TRANSACTION;"
		_, err := DB.Exec(s)
		if err != nil {
			DB.Exec("ROLLBACK TRANSACTION;")
			return err
		}
	}
	return nil
}

//DeleteTableData изменяет таблицу tabname согласно условию w
func DeleteTableData(tabname string, w map[string]string) error {
	cond := ""
	where := ""
	for k, v := range w {
		where = where + cond + k + Escape(v)
		cond = " and "
	}
	s := "DELETE FROM " + tabname + " WHERE " + where
	_, err := DB.Exec(s)
	if err != nil {
		DB.Exec("ROLLBACK TRANSACTION;")
		return err
	}
	return nil
}

//GetCatalog вернет json дерево номенклатуры для отображения в шаблоне
func GetCatalog() (string, error) {
	var s string
	type catalog struct {
		uid    string
		name   string
		art    string
		group  string
		grname string
		icon   string
	}
	var gds catalog
	/*
			[
		  {
		    text: "Parent 1",
		    nodes: [
		      {
		        text: "Child 1",
		        nodes: [
		          {
		            text: "Grandchild 1"
		          },
		          {
		            text: "Grandchild 2"
		          }
		        ]
		      },
		      {
		        text: "Child 2"
		      }
		    ]
		  },
	*/
	s = "select g.uid, g.name as name, g.art as art, g.groupname as code,ifnull(gr.name,'') as grname, ifNULL(gr.icon,'') as icon from goods as g left join groups as gr on g.groupname=gr.code order by g.groupname;"
	rows, err := DB.Query(s)
	if err != nil {
		return "[]", err
		//log.Panic(err)
	}
	defer rows.Close()
	var prevgr string
	var icon string
	var nodes string
	var cat string
	catcomma := ""
	comma := ""
	for rows.Next() {
		err := rows.Scan(&gds.uid, &gds.name, &gds.art, &gds.group, &gds.grname, &gds.icon)
		if err != nil {
			rows.Close()
			return "[]", err
		}
		if prevgr != gds.group {
			cat = cat + catcomma + "{text:" + Escape(prevgr) + ",icon:" + icon + ",nodes:[" + nodes + "]}"
			prevgr = gds.group
			nodes = ""
			comma = ""
			catcomma = ","
		}
		nodes = nodes + comma + "{ text:" + Escape(gds.name) + "}"
		comma = ","
	}
	if len(nodes) > 0 {
		cat = cat + catcomma + "{text:" + Escape(prevgr) + ",icon:" + icon + ",nodes:[" + nodes + "]}"
	}

	return "[" + cat + "]", nil

}

//GetConfig возвращает мап конфигурации
func GetConfig() (Config, error) {
	var c = make(Config)
	rows, err := DB.Query("select name, value from config;")
	if err != nil {
		return c, err
		//log.Panic(err)
	}
	defer rows.Close()
	var value string
	var name string
	for rows.Next() {
		err := rows.Scan(&name, &value)
		if err != nil {
			return c, err
		}
		c[name] = value
	}
	return c, nil

}

//Save сохраняет мап конфигурации
func (c Config) Save() (string, error) {
	var s = make([]string, 0, len(c))
	for k, v := range c {
		rows, err := DB.Query("select value from config where name=$1;", k)
		if err != nil {
			return err.Error(), err
		}
		var value string
		if rows.Next() {
			err := rows.Scan(&value)
			if err != nil {
				return err.Error(), err
			}
			if value == v {
				rows.Close()
				continue
			}
			s = append(s, "update config set value="+Escape(v)+" where name="+Escape(k)+";")
		} else {
			s = append(s, "insert into config (name, value) values("+Escape(k)+","+Escape(v)+");")
		}
		rows.Close()
	}
	var slog string
	for _, v := range s {
		res, err := DB.Exec(v)
		if err != nil {
			return v, err
		}
		affect, _ := res.RowsAffected()
		slog = slog + strconv.FormatInt(affect, 10) + " " + v + ","
	}

	return slog, nil
}

//ValInt возврат целого числа
func (c Config) ValInt(key string, def int) int {
	var ret int
	var err error
	i, ok := c[key]
	if !ok {
		ret = def
	} else {
		ret, err = strconv.Atoi(i)
		if err != nil {
			ret = def
		}
	}
	return ret
}

//ValF64 возврат float64
func (c Config) ValF64(key string, def float64) float64 {
	var ret float64
	var err error
	i, ok := c[key]
	if !ok {
		ret = def
	} else {
		ret, err = strconv.ParseFloat(i, 64)
		if err != nil {
			ret = def
		}
	}
	return ret
}

//ValString возврат строки
func (c Config) ValString(key string, def string) string {
	var ret string
	i, ok := c[key]
	if !ok {
		ret = def
	} else {
		ret = strings.Trim(i, " ")
	}
	return ret
}

// GetMagNames возвращает срез мапов из таблицы магазинов. tip-тип склада. В выборку попадают склады равно или выше значения tip
func GetMagNames(tip int, uidStore string) (*Stores, error) {
	st := make(Stores)
	var q string
	var z = len(uidStore)
	if tip <= -50 {
		q = "select uid, name from stores where tip=$1;"
		tip = tip + 100
	} else {
		q = "select uid, name from stores where tip>=$1;"
	}
	rows, err := DB.Query(q, tip) //where name like '%рдж%'
	if err != nil {
		return &st, err
		//log.Panic(err)
	}
	defer rows.Close()
	var uid string
	var name string
	for rows.Next() {
		err := rows.Scan(&uid, &name)
		if err != nil {
			return &st, err
		}
		if z > 1 {
			if uidStore == uid {
				st[uid] = name
			}
		} else {
			st[uid] = name
		}
	}
	return &st, nil

}

// GetContracts возвращает таблицу контрактов с поставщиками.
func GetContracts(r ...string) ([]Contract, error) {
	st := Contract{}
	ct := make([]Contract, 0, 128)
	var rows *sql.Rows
	var err error
	switch len(r) {
	case 0:
		rows, err = DB.Query("select provider,recipient,chedord,cheddeliv,delivdays from contracts where autoord=1;")
	case 1:
		rows, err = DB.Query("select provider,recipient,chedord,cheddeliv,delivdays from contracts where autoord=1 and recipient=$1;", r[0])
	case 2:
		rows, err = DB.Query("select provider,recipient,chedord,cheddeliv,delivdays from contracts where autoord=1 and recipient=$1 and provider=$2;", r[0], r[1])
	}
	if err != nil {
		return nil, err
		//log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&st.Provider, &st.Recipient, &st.Chedord, &st.Cheddeliv, &st.Delivdays)
		if err != nil {
			return ct[0:], err
		}
		ct = append(ct, st)
	}
	return ct[0:], nil

}

//GetGood возвращает срез мапов из таблицы товаров.
func GetGood(guid string) (*Goods, error) {
	st := new(Goods)
	rows, err := DB.Query("select uid, groupname, name, art from goods where uid =$1;", guid)
	if err != nil {
		return st, err
		//log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		err := rows.Scan(&st.KeyGoods, &st.Grp, &st.Name, &st.Art)
		if err != nil {
			return st, err
		}
	}
	return st, nil

}

//GetGoods возвращает срез мапов из таблицы товаров.
func GetGoods(q string) ([]Goods, error) {
	var st Goods
	var gds = make([]Goods, 0)
	if len(q) == 0 {
		return gds, nil
	}
	rows, err := DB.Query("select uid, groupname, name, art from goods where art like $1;", q+"%")
	if err != nil {
		return gds, err
		//log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&st.KeyGoods, &st.Grp, &st.Name, &st.Art)
		if err != nil {
			return gds, err
		}
		gds = append(gds, st)
	}
	if len(gds) > 0 {
		return gds, nil
	}
	//не нашли по артиклу,ищем по наименованию
	rows.Close()
	rows, err = DB.Query("select uid, groupname, name, art from goods where name like $1;", "%"+q+"%")
	for rows.Next() {
		err := rows.Scan(&st.KeyGoods, &st.Grp, &st.Name, &st.Art)
		if err != nil {
			return gds, err
		}
		gds = append(gds, st)
	}
	return gds, nil

}

//CreateGoods возвращает срез мапов из таблицы товаров.
func CreateGoods(g *Goods) (int64, error) {

	res, err := DB.Exec("INSERT OR REPLACE INTO goods (uid, groupname, name, art) values($1,$2,$3,$4) ;", g.KeyGoods, g.Grp, g.Name, g.Art)
	if err != nil {
		return 0, err
		//log.Panic(err)
	}
	lastid, _ := res.LastInsertId()
	return lastid, nil

}

//GetSales возвращает продажи из таблицы goodsmov
func GetSales(kStore string, kGoods string, p ...string) (*Sales, error) {
	var p1, p2 string
	switch len(p) {
	case 0:
		p1 = "2000-01-01"
		p2 = "date('now')"
	case 1:
		p1 = p[0]
		p2 = "date('now')"
	default:
		p1 = p[0]
		p2 = p[1]
	}
	s := new(Sales)
	s.KeyStore = kStore
	s.KeyGoods = kGoods
	//CREATE TABLE goodsmov (id integer PRIMARY KEY, uidStore text NOT NULL, uidGoods text NOT NULL, groupGoods text, period text NOT NULL, cnt real, summa integer, margin real, balance real, prevdays integer, zerodays integer
	rows, err := DB.Query("select CAST( strftime('%s', m.period)/86400 as Integer) as uprd, m.cnt as cnt , m.prevdays as pd, m.period, m.balance, m.margin, m.summa, m.zerodays, CASE m.tipmov WHEN 'S' THEN 0 ELSE 1 END as tipmov from goodsmov m where m.uidStore=$1 and m.uidGoods=$2 and m.period>=$3 and m.period<=$4 order by m.period;", kStore, kGoods, p1, p2)
	if err != nil {
		return s, err
		//log.Panic(err)
	}
	defer rows.Close()
	gm := goodsmov{}
	for rows.Next() {
		err := rows.Scan(&gm.udate, &gm.cnt, &gm.prevdays, &gm.period, &gm.balance, &gm.margin, &gm.summa, &gm.zerodays, &gm.tipmov)
		if err != nil {
			return s, err
		}
		//пишем только продажи
		if gm.tipmov != 1 {
			if gm.udate.Valid {
				s.Udate = append(s.Udate, gm.udate.Float64)
			} else {
				s.Udate = append(s.Udate, 0.0)
			}
			if gm.cnt.Valid {
				s.Cnt = append(s.Cnt, gm.cnt.Float64)
			} else {
				s.Cnt = append(s.Cnt, 0.0)
			}
			if gm.prevdays.Valid {
				s.Prevdays = append(s.Prevdays, gm.prevdays.Float64)
			} else {
				s.Prevdays = append(s.Prevdays, 0.0)
			}
			if gm.balance.Valid {
				s.Balance = append(s.Balance, gm.balance.Float64)
			} else {
				s.Balance = append(s.Balance, 0.0)
			}
			if gm.margin.Valid {
				s.Margin = append(s.Margin, gm.margin.Float64)
			} else {
				s.Margin = append(s.Margin, 0.0)
			}
			if gm.summa.Valid {
				s.Summa = append(s.Summa, gm.summa.Float64)
			} else {
				s.Summa = append(s.Summa, 0.0)
			}
			if gm.zerodays.Valid {
				s.Zdays = append(s.Zdays, gm.zerodays.Float64)
			} else {
				s.Zdays = append(s.Zdays, 0.0)
			}
		}
		if gm.udate.Valid {
			s.LastPeriod = gm.udate.Float64
		} else {
			s.LastPeriod = 0.0
		}
		if gm.balance.Valid {
			s.LastBalance = gm.balance.Float64
		} else {
			s.LastBalance = 0.0
		}
	}

	return s, nil
}

//GetLastSales получает статистику продаж по складу uidStore для товара uidGoods
func GetLastSales(uidStore string, uidGoods string) (string, string, error) {
	//rows, err := DB.Query("select uidGoods from goodsmov where uidStore=$1 and uidGoods=$2 and period=$3;", uidStore, uidGoods, period)
	rows, err := DB.Query("select g.period, g.tipmov from goodsmov as g left join stores as s on g.uidStore=s.uid WHERE uidStore=$1 and uidGoods=$2 and g.tipmov=(CASE WHEN s.tip=0 THEN 'M' ELSE 'S' END) order by period DESC limit 1;", uidStore, uidGoods)
	if err != nil {
		return "1970-01-01", "M", err
		//log.Panic(err)
	}
	defer rows.Close()
	var lastper string
	var tipmov string
	if rows.Next() {
		err := rows.Scan(&lastper, &tipmov)
		if err != nil {
			return "1970-01-01", "M", err
		}
	} else {
		lastper = "1970-01-01"
		tipmov = "M"
	}
	return lastper, tipmov, nil
}

//SaveSales сохраняет данные в базу
func SaveSales(uidStore string, uidGoods string, period string, tipmov string, cnt float64, summa float64, margin float64, balance float64, prevdays int, zerodays int) error {
	//rows, err := DB.Query("select uidGoods from goodsmov where uidStore=$1 and uidGoods=$2 and period=$3;", uidStore, uidGoods, period)
	_, err := DB.Exec("DELETE from goodsmov WHERE uidStore=$1 and uidGoods=$2 and period=$3;", uidStore, uidGoods, period)
	if err != nil {
		return err
	}
	//defer rows.Close()
	_, err = DB.Exec("INSERT INTO goodsmov (uidStore,uidGoods, period,tipmov, cnt, summa, margin, balance, prevdays,zerodays) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", uidStore, uidGoods, period, tipmov, cnt, summa, margin, balance, prevdays, zerodays)
	if err != nil {
		return err
		//log.Panic(err)
	}
	return nil
}

//InsRepSales изменяет таблицу продажи товаров
func InsRepSales(matr []map[string]interface{}, keyfordel map[string]string) error {
	return InsertTableData("goodsmov", matr, keyfordel)
}

//GetGoodsFromMatrix возвращает матрицу товаров
func GetGoodsFromMatrix(kStore string) (st []string, err error) {
	//rows, err := DB.Query("select m.uidGoods, ifnull(p.period,'1970-01-01') , ifnull(p.cnt,0), ifnull(p.days,0), ifnull(p.demand,0) from salesmatrix as m left join predict as p on m.uidStore=p.uidStore and m.uidgoods=p.uidgoods where m.uidStore=$1 and inuse=1;", kStore)
	rows, err := DB.Query("select uidGoods from salesmatrix where uidStore=$1 and inuse=1;", kStore) // uidGoods='ea716efd-52f8-11e5-ad24-3085a9a9595a' and
	if err != nil {
		return st, err
		//log.Panic(err)
	}
	defer rows.Close()
	var s []byte
	for rows.Next() {
		err := rows.Scan(&s)
		if err != nil {
			return st, err
		}
		st = append(st, string(s))
	}
	return st, nil
}

//GetAllGoodsFromMatrix возвращает матрицу товаров
func GetAllGoodsFromMatrix(kStore string, kGoods string) (mg []MatrixGoods, err error) {
	//rows, err := DB.Query("select uidGoods, minbalance, maxbalance, abc, vitrina from salesmatrix where uidStore=$1;", kStore) // uidGoods='ea716efd-52f8-11e5-ad24-3085a9a9595a' and
	var rows *sql.Rows
	if len(kGoods) == 0 {
		rows, err = DB.Query(`select s.uidGoods, s.minbalance, s.maxbalance, ifnull(s.abc,'C') as abc, s.vitrina, ifnull(zz.balance,0) as balance, ifnull(p.period,'1970-01-01') as predictper , ifnull(p.cnt,0) as predcnt, ifnull(p.days,0) as preddays, ifnull(p.demand,0) as preddemand, s.step from salesmatrix s LEFT JOIN 
	(select z.uidgoods as uidgoods, z.balance as balance, z.period from goodsmov as z join (select max(g.id) as id from goodsmov as g where g.uidStore=$1 group by g.uidStore, g.uidgoods) as a on a.id=z.id) as zz
	on s.uidGoods=zz.uidgoods left join 
	(select p.uidStore, p.uidgoods, p.period, p.cnt, p.days, p.demand from predict as p join 
	(select max(period) as mperiod, max(id) as id,uidStore, uidgoods from predict where uidStore=$1 group by uidStore, uidgoods) as p1
	on p.id=p1.id where p.uidStore=$1) as p on s.uidStore=p.uidStore and s.uidgoods=p.uidgoods where s.uidStore=$1 and s.inuse=1;`, kStore)
	} else {
		rows, err = DB.Query(`select s.uidGoods, s.minbalance, s.maxbalance, ifnull(s.abc,'C') as abc, s.vitrina, ifnull(zz.balance,0) as balance, ifnull(p.period,'1970-01-01') as predictper , ifnull(p.cnt,0) as predcnt, ifnull(p.days,0) as preddays, ifnull(p.demand,0) as preddemand, s.step from salesmatrix s LEFT JOIN 
	(select z.uidgoods as uidgoods, z.balance as balance, z.period from goodsmov as z join (select max(g.id) as id from goodsmov as g where g.uidStore=$1 and g.uidgoods=$2 group by g.uidStore, g.uidgoods) as a on a.id=z.id) as zz
	on s.uidGoods=zz.uidgoods left join 
	(select p.uidStore, p.uidgoods, p.period, p.cnt, p.days, p.demand from predict as p join 
	(select max(period) as mperiod, max(id) as id,uidStore, uidgoods from predict where uidStore=$1 and uidgoods=$2 group by uidStore, uidgoods) as p1
	on p.id=p1.id where p.uidStore=$1 and p.uidgoods=$2) as p on s.uidStore=p.uidStore and s.uidgoods=p.uidgoods where s.uidStore=$1 and s.uidgoods=$2 and s.inuse=1;`, kStore, kGoods)
	}
	lmg := MatrixGoods{}
	mg = make([]MatrixGoods, 0, 250)
	if err != nil {
		return mg, err
		//log.Panic(err)
	}
	defer rows.Close()
	var nf sql.NullFloat64
	var nfd sql.NullFloat64
	var ns sql.NullString
	var nsp sql.NullString
	for rows.Next() {
		err := rows.Scan(&lmg.KeyGoods, &lmg.MinBalance, &lmg.MaxBalance, &ns, &lmg.Vitrina, &nf, &nsp, &lmg.PredCnt, &lmg.PredDays, &nfd, &lmg.Step)
		if err != nil {
			return mg, err
		}
		if nf.Valid {
			lmg.Balance = nf.Float64
		} else {
			lmg.Balance = 0.0
		}
		if nfd.Valid {
			lmg.PredDemand = nfd.Float64
		} else {
			lmg.PredDemand = 0.0
		}
		if ns.Valid {
			lmg.Abc = ns.String
		} else {
			lmg.Abc = "C"
		}
		if nsp.Valid {
			lmg.PredPeriod = nsp.String
		} else {
			lmg.PredPeriod = "1970-01-01"
		}
		mg = append(mg, lmg)
	}
	return mg, nil
}

//UpdateMatrix изменяет таблицу матрицы товаров
func UpdateMatrix(m map[string]interface{}, w map[string]string) error {
	matr := make([]map[string]interface{}, 0, 1)
	matr = append(matr, m)
	return UpdateTableData("salesmatrix", matr, w)
	/*
		s := "update salesmatrix "
		comma := ""
		for k, v := range m {
			switch v.(type) {
			case float64:
				s = s + comma + " set  " + k + "=" + strconv.FormatFloat((v.(float64)), 'f', -1, 64)
			case int64:
				s = s + comma + " set  " + k + "=" + strconv.FormatInt((v.(int64)), 10)
			case int:
				s = s + comma + " set  " + k + "=" + strconv.FormatInt(int64(v.(int)), 10)
			case bool:
				if (v.(bool)) == true {
					s = s + comma + " set  " + k + "= true"
				} else {
					s = s + comma + " set  " + k + "= false"
				}
			case string:
				s = s + comma + " set  " + k + "= " + escstr(v.(string))
			default:
				s = s + comma + " set  " + k + "= " + escstr(v.(string))
			}
			comma = ","
		}
		s = s + " where "
		cond := ""
		for k, v := range w {
			s = s + cond + k + "=" + escstr(v)
			cond = " and "
		}
		_, err := DB.Exec(s)
		if err != nil {
			return err
		}
		return nil
	*/
}

//ReplaceMatrix изменяет таблицу матрицы товаров
func ReplaceMatrix(matr []map[string]interface{}, w map[string]string) error {
	return InsertTableData("salesmatrix", matr, w)
	/*
		itrans := 500
		s := ""
		i := 0
		for _, m := range matr {
			flds := ""
			val := ""
			comma := ""
			for k, v := range m {
				flds = flds + comma + k
				switch v.(type) {
				case float64:

					val = val + comma + strconv.FormatFloat((v.(float64)), 'f', -1, 64)
				case int64:
					val = val + comma + strconv.FormatInt((v.(int64)), 10)
				case int:
					val = val + comma + strconv.FormatInt(int64(v.(int)), 10)
				case bool:
					if (v.(bool)) == true {
						val = val + comma + " true"
					} else {
						val = val + comma + "false"
					}
				case string:
					val = val + comma + escstr(v.(string))
				}
				comma = ","
			}
			s = s + "INSERT OR REPLACE INTO salesmatrix (" + flds + ") VALUES(" + val + ");"
			i++
			if i > itrans {
				i = 0
				s = "BEGIN TRANSACTION;" + s + "COMMIT TRANSACTION;"
				_, err := DB.Exec(s)
				if err != nil {
					return err
				}
				s = ""
			}
		}
		if s != "" {
			s = "BEGIN TRANSACTION;" + s + "COMMIT TRANSACTION;"
			_, err := DB.Exec(s)
			if err != nil {
				return err
			}
		}
		return nil
	*/
}

//GetProfit возвращвет прибыль для магазина uidStore
func GetProfit(uidStore string, pfrom string, pto string) (map[string]float64, float64) {
	goods := make(map[string]float64)
	var v float64
	var kol float64
	var uid string
	var sum float64 = 0.0
	rows, err := DB.Query("select uidgoods, sum(margin*summa) as prib, sum(cnt) as kol from goodsmov where uidStore=$1 and period>$2 and period<$3 and tipmov='S' group by uidStore, uidgoods having prib>0 order by prib DESC;", uidStore, pfrom, pto)
	if err != nil {
		return goods, sum
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&uid, &v, &kol)
		if err != nil {
			return goods, sum
		}
		goods[uid] = v
		sum = sum + v
	}
	return goods, sum
}

//MakeSalesMatrix собирает матрицу товаров для магазина по итогам продаж
func MakeSalesMatrix() error {
	//CREATE TABLE goodsmov (id integer PRIMARY KEY, uidStore text NOT NULL, uidGoods text NOT NULL, groupGoods text, period text NOT NULL, cnt real, summa integer, margin real, balance real, prevdays integer, zerodays integer
	_, err := DB.Exec("insert into salesmatrix (uidStore,uidGoods,minBalance, maxBalance,vitrina,cost) select m.uidStore, m.uidGoods, CASE WHEN julianday(max(m.period))-julianday(min(m.period)) <= 10 AND julianday('now')-julianday(min(m.period)) <50 THEN 1 WHEN julianday(max(m.period))-julianday(min(m.period)) <= 10 THEN 0 ELSE 1 END minBalance, CASE WHEN julianday(max(m.period))-julianday(min(m.period)) > 10 THEN CAST(0.5+count(m.cnt)*30/(julianday(max(m.period))-julianday(min(m.period))) AS INTEGER) ELSE 0 END as maxBalance, 0 as vitrin, 100 as cost from goodsmov m GROUP BY m.uidStore, m.uidGoods;")
	if err != nil {
		return err
		//log.Panic(err)
	}
	return nil
}

//SaveRbfNet сохраняет данные весов сети в базу
func SaveRbfNet(uidstore string, uidgoods string, datanet string, uperiod float64, sigma float64) error {
	//CREATE UNIQUE INDEX storegoods ON neuro (uidStore, uidGoods)
	_, err := DB.Exec("INSERT OR REPLACE INTO neuro (uidStore, uidGoods, netdata, period, sigmaper) VALUES($1,$2,$3,$4,$5);", uidstore, uidgoods, datanet, time.Unix(int64(uperiod*86400), 0).Format("2006-01-02"), sigma)
	if err != nil {
		return err
		//log.Panic(err)
	}
	return nil
}

//LoadRbfNet получает данные весов сети из базы
func LoadRbfNet(uidstore string, uidgoods string) (rbf *Neuro, err error) {
	rbf = &Neuro{}
	rbf.KeyGoods = uidgoods
	rbf.KeyStore = uidstore
	//CREATE UNIQUE INDEX storegoods ON neuro (uidStore, uidGoods)
	rows, err := DB.Query("select netdata, period, sigmaper from neuro  where uidStore=$1 and uidGoods=$2;", uidstore, uidgoods)
	if err != nil {
		return rbf, err
		//log.Panic(err)
	}
	defer rows.Close()
	var nullfloat sql.NullFloat64
	if rows.Next() {
		err := rows.Scan(&rbf.Netdata, &rbf.Period, &nullfloat)
		if err != nil {
			return rbf, err
		}
		if nullfloat.Valid {
			rbf.SigmaPer = nullfloat.Float64
		} else {
			rbf.SigmaPer = 0.0
		}
	}
	return rbf, nil
}

//SavePredict сохраняет данные предсказаний количества покупок pred за days дней для магазина uidstore, товара uidgoods
func SavePredict(uidstore string, uidgoods string, pred float64, period float64, days int, demand float64) error {
	//CREATE UNIQUE INDEX storegoodsperiod ON predict (uidStore,uidGoods,period)
	_, err := DB.Exec("INSERT OR REPLACE INTO predict (uidStore, uidGoods, period, cnt, days, demand) VALUES($1,$2,$3,$4,$5,$6);", uidstore, uidgoods, time.Unix(int64(period*86400), 0).Format("2006-01-02"), int(pred+0.5), days, demand)
	if err != nil {
		return err
		//log.Panic(err)
	}
	return nil
}

//GetLastPredict получает данные предсказаний количества покупок pred за days дней для магазина uidstore, товара uidgoods
func GetLastPredict(uidstore string, uidgoods string) (pr *Predict, err error) {
	pr = &Predict{}
	pr.KeyGoods = uidgoods
	pr.KeyStore = uidstore
	pr.Period = "1970-01-01"
	pr.Days = 0
	//CREATE UNIQUE INDEX storegoods ON neuro (uidStore, uidGoods)
	rows, err := DB.Query("select period, cnt, days, demand from predict where uidStore=$1 and uidGoods=$2 order by period DESC;", uidstore, uidgoods)
	if err != nil {
		return pr, err
		//log.Panic(err)
	}
	defer rows.Close()
	var nf sql.NullFloat64
	var nfd sql.NullFloat64
	if rows.Next() {
		err := rows.Scan(&pr.Period, &nf, &pr.Days, &nfd)
		if err != nil {
			return pr, err
		}
		if nf.Valid {
			pr.Cnt = nf.Float64
		} else {
			pr.Cnt = 0.0
		}
		if nfd.Valid {
			pr.Demand = nfd.Float64
		} else {
			pr.Demand = 0.0
		}
	}
	return pr, nil
}

//DbLog сохраняет лог в базу
func DbLog(tlog string, tfunc string, n int64) error {
	//CREATE UNIQUE INDEX storegoodsperiod ON predict (uidStore,uidGoods,period)
	_, err := DB.Exec("INSERT INTO log (period, log, func, nano) VALUES($1,$2,$3,$4);", time.Now().Format("2006-01-02T15:04:05"), tlog, tfunc, int(n))
	if err != nil {
		return err
		//log.Panic(err)
	}
	return nil
}

//GetLastStateNetwork читает из лога num последних записей
func GetLastStateNetwork(num int, strmodul string) map[int]string {
	var l = make(map[int]string)
	var p int
	var s string
	if num == 0 {
		num = 3
	}
	var err error
	var rows *sql.Rows
	//rows, err := DB.Query("select CAST(strftime('%s', k.period)/86400 as Integer) as p, k.log, k.nano from log k where func='calculate' order by nano DESC Limit $1;", num)
	if len(strmodul) > 0 {
		rows, err = DB.Query("select k.nano as p, k.log from log k where func=$1 order by id DESC Limit $2;", strmodul, num)
	} else {
		rows, err = DB.Query("select k.nano as p, k.log from log k order by id DESC Limit $1;", num)
	}
	if err != nil {
		return l
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&p, &s)
		if err != nil {
			return l
		}
		l[p] = s
	}
	return l
}

//SaveOper сохраняет данные заказов в базу
func SaveOper(numdoc string, provider string, uidstore string, uidgoods string, period string, cnt float64, nextper string, delivery string) error {
	//если заказ уже сделан то пропускаем
	needwrite := true
	rows, err := DB.Query("Select cnt from oper where provider=$1 and uidStore=$2 and delivdays>=$3", provider, uidstore, period)
	if err == nil {
		var nf sql.NullFloat64
		if rows.Next() {
			err := rows.Scan(&nf)
			if err == nil {
				if nf.Valid {
					cnt = nf.Float64 - cnt
					needwrite = false
				}
			}

		}
		rows.Close()
	}
	if needwrite && cnt > 0 {
		_, err := DB.Exec("INSERT OR REPLACE INTO oper (uidStore, uidGoods, provider, period, cnt, nextper,NumDoc,delivery) VALUES($1,$2,$3,$4,$5,$6,$7,$8);", uidstore, uidgoods, provider, period, cnt, nextper, numdoc, delivery)
		if err != nil {
			return err
			//log.Panic(err)
		}
	}
	return nil
}

//GetZakaz получает данные заказов
func GetZakaz(period string) ([]Zakaz, error) {
	var zaks = make([]Zakaz, 0)
	var zg = make([]ZakazGoods, 0)

	var err error
	var rows *sql.Rows
	if period == "last" {
		rows, err = DB.Query("Select period from oper ORDER BY period DESC Limit 1;")
		if err != nil {
			return zaks, err
			//log.Panic(err)
		}
		var p string
		if rows.Next() {
			err = rows.Scan(&p)
			if err != nil {
				rows.Close()
				return zaks, err
			}
		}
		rows.Close()
		//t, _ := time.Parse("2006-01-02T", p)
		period = p //t.Format("2006-01-02")
	}
	rows, err = DB.Query("Select uidStore, uidGoods, provider, period, cnt, nextper, NumDoc from oper WHERE period>=$1 ORDER BY NumDoc,provider,uidStore;", period)
	if err != nil {
		return zaks, err
		//log.Panic(err)
	}
	defer rows.Close()
	var store, goods, provider, pr, nextper, numdoc, prevnum, prevprov, prevstore string
	var cnt sql.NullFloat64
	z := Zakaz{}
	i := ZakazGoods{}
	for rows.Next() {
		err := rows.Scan(&store, &goods, &provider, &pr, &cnt, &nextper, &numdoc)
		if err != nil {
			return zaks, err
		}

		if len(zaks) != 0 && (prevprov != provider || prevstore != store || prevnum != numdoc) {
			//новый док
			z.Items = zg
			zaks = append(zaks, z)
			z = Zakaz{}
			z.Period = pr
			z.DelivPeriod = nextper
			z.Num = numdoc
			z.Provider = provider
			z.Recipient = store
			i = ZakazGoods{}
			zg = make([]ZakazGoods, 0)
		}
		if z.Provider == "" {
			//z = Zakaz{}
			z.Period = pr
			z.DelivPeriod = nextper
			z.Num = numdoc
			z.Provider = provider
			z.Recipient = store
			zg = make([]ZakazGoods, 0)
		}
		i = ZakazGoods{}
		if cnt.Valid {
			i.Cnt = cnt.Float64
		} else {
			i.Cnt = 0.0
		}
		i.Price = 0.0
		i.Gds = goods
		zg = append(zg, i)
		prevnum = numdoc
		prevprov = provider
		prevstore = store
	}
	z.Items = zg
	zaks = append(zaks, z)
	return zaks, nil
}

//GetZakazXML получает данные заказов
func GetZakazXML(period string) ([]OrderXML, error) {
	var orders = make([]OrderXML, 0)
	var items = make([]ItemXML, 0)

	var err error
	var rows *sql.Rows
	if period != "last" {
		t, err := time.Parse("2006-01-02T15:04:05", period)
		if err != nil {
			//формат даты другой
			t, err = time.Parse("2006-01-02", period)
			if err != nil {
				period = "last"
			} else {
				period = t.Format("2006-01-02")
			}
		} else {
			period = t.Format("2006-01-02")
		}
	}
	if period == "last" {
		rows, err = DB.Query("Select period from oper ORDER BY period DESC Limit 1;")
		if err != nil {
			return orders, err
			//log.Panic(err)
		}
		var p string
		if rows.Next() {
			err = rows.Scan(&p)
			if err != nil {
				rows.Close()
				return orders, err
			}
		}
		rows.Close()
		period = p
	}

	rows, err = DB.Query("Select uidStore, uidGoods, provider, period, cnt, nextper, NumDoc from oper WHERE date(period)=date($1) ORDER BY NumDoc,provider,uidStore;", period)
	if err != nil {
		return orders, err
		//log.Panic(err)
	}
	defer rows.Close()
	var store, goods, provider, pr, nextper, numdoc, prevnum, prevprov, prevstore string
	var cnt sql.NullFloat64
	order := OrderXML{}
	item := ItemXML{}
	itemsxml := ItemsXML{}
	for rows.Next() {
		err := rows.Scan(&store, &goods, &provider, &pr, &cnt, &nextper, &numdoc)
		if err != nil {
			return orders, err
		}

		if order.Provider != "" && (prevprov != provider || prevstore != store || prevnum != numdoc) {
			//новый док
			itemsxml = ItemsXML{Items: items}
			order.Items = itemsxml
			orders = append(orders, order)
			order = OrderXML{}
			order.Period = pr
			order.DelivPeriod = nextper
			order.Num = numdoc
			order.Provider = provider
			order.Recipient = store
			items = make([]ItemXML, 0)
		}
		//для первой итерации
		if order.Provider == "" {
			//z = Zakaz{}
			order.Period = pr
			order.DelivPeriod = nextper
			order.Num = numdoc
			order.Provider = provider
			order.Recipient = store
			items = make([]ItemXML, 0)
		}
		item = ItemXML{}
		if cnt.Valid {
			item.Cnt = float64(int(cnt.Float64 + 0.5))
		} else {
			item.Cnt = 0.0
		}
		item.Price = 0.0
		item.Gds = goods
		items = append(items, item)
		prevnum = numdoc
		prevprov = provider
		prevstore = store
	}
	itemsxml = ItemsXML{Items: items}
	order.Items = itemsxml
	orders = append(orders, order)
	return orders, nil
}
