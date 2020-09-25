package main

import (
	"strconv"
	"strings"
	"time"
)

//CheckDate проверяет дату на валидность format-go шный формат даты date
func checkDate(format, date string) bool {
	t, err := time.Parse(format, date)
	if err != nil {
		return false
	}
	return t.Format(format) == date
}

//inChedule проверяет на вхождение даты в формат cron
func inChedule(ched string, t ...time.Time) bool {
	/*
									0 12 * * 3
		MIN	Минуты	От 0 до 59 -----|
		HOUR	Часы	От 0 до 23 ---|
		DOM	День месяца	1-31 -----------|
		MON	Месяц	1-12  ----------------|
		DOW	День недели	0-6 ----------------|
	*/
	arr := strings.Fields(ched)
	if len(arr) < 3 {
		return false
	}
	now := time.Now()
	//предтдущее событие
	var pred time.Time
	if len(t) > 0 {
		now = t[0]
	}
	nowdm := now.Day()
	nowdw := int(now.Weekday())
	if nowdw == 0 {
		nowdw = 7
	}
	nowmon := int(now.Month())
	var okdw, okmon, okdm bool
	dw := arr[len(arr)-1]
	mon := arr[len(arr)-2]
	dm := arr[len(arr)-3]
	//парсим дни недели
	if strings.Contains(dw, "*") {
		okdw = true
	} else {
		//посмотрим надо ли повторять событие через определенное количество недель
		//например 0 12 * * 3/2 будет пытаться повторить событие каждую вторую среду,
		//если конечно указана дата предидущего события
		//в то же время запись 0 12 * * 1-5/2 означает повторять с пн по пт через день, т.е пн,ср,пт
		kof := 1
		if strings.Contains(dw, "/") {
			v, err := strconv.Atoi(dw[1+strings.Index(dw, "/"):])
			if err == nil {
				kof = v
			}
			dw = dw[:strings.Index(dw, "/")]
		}
		if strings.Contains(dw, ",") {
			if strings.Index(dw, strconv.Itoa(nowdw)) >= 0 { //dw = 0:7
				okdw = true
			}
		} else {
			if strings.Contains(dw, "-") {
				arr = strings.Split(dw, "-")
				beg, err := strconv.Atoi(arr[0])
				if err != nil {
					beg = 0
				}
				end, err := strconv.Atoi(arr[1])
				if err != nil {
					end = 7
				}
				for i := 0; i <= end; i++ {
					if beg+kof*i == nowdw && beg+kof*i <= end {
						okdw = true
					}
				}
			} else {
				//если не указана дата предидущего события, то коэф. всегда 1, т.е. повторения через / не учитываются
				if len(t) == 2 {
					pred = t[1]
				} else {
					kof = 1
				}
				v, err := strconv.Atoi(dw)
				if err == nil && v == nowdw {
					if kof == 1 {
						okdw = true
					} else {
						if pred.AddDate(0, 0, 7*kof).YearDay() == now.YearDay() {
							okdw = true
						}
					}
				}
			}
		}
	}
	//месяц
	if strings.Contains(mon, "*") {
		okmon = true
	} else {
		if strings.Contains(mon, ",") {
			arr = strings.Split(mon, ",")
			for _, v := range arr {
				i, err := strconv.Atoi(v)
				if err == nil {
					if i == nowmon {
						okmon = true
						break
					}
				}
			}
		} else {
			if strings.Contains(mon, "-") {
				kof := 1
				if strings.Contains(mon, "/") {
					v, err := strconv.Atoi(mon[1+strings.Index(mon, "/"):])
					if err == nil {
						kof = v
					}
					mon = mon[:strings.Index(mon, "/")]
				}
				arr = strings.Split(mon, "-")
				beg, err := strconv.Atoi(arr[0])
				if err != nil {
					beg = 1
				}
				end, err := strconv.Atoi(arr[1])
				if err != nil {
					end = 12
				}
				for i := 0; i <= end && beg+kof*i <= end; i++ {
					if beg+kof*i == nowmon {
						okmon = true
					}
				}
			} else {
				v, err := strconv.Atoi(mon)
				if err == nil && v == nowmon {
					okmon = true
				}
			}
		}
	}
	//день месяца
	if strings.Contains(dm, "*") {
		okdm = true
	} else {
		if strings.Contains(dm, ",") {
			arr = strings.Split(dm, ",")
			for _, v := range arr {
				i, err := strconv.Atoi(v)
				if err == nil {
					if i == nowdm {
						okdm = true
						break
					}
				}
			}
		} else {
			if strings.Contains(dm, "-") {
				kof := 1
				if strings.Contains(dm, "/") {
					v, err := strconv.Atoi(dm[1+strings.Index(dm, "/"):])
					if err == nil {
						kof = v
					}
					dm = dm[:strings.Index(dm, "/")]
				}
				arr = strings.Split(dm, "-")
				beg, err := strconv.Atoi(arr[0])
				if err != nil {
					beg = 1
				}
				end, err := strconv.Atoi(arr[1])
				if err != nil {
					end = 31
				}
				for i := 0; i <= end && beg+kof*i <= end; i++ {
					if beg+kof*i == nowdm {
						okdm = true
					}
				}
			} else {
				v, err := strconv.Atoi(dm)
				if err == nil && v == nowdm {
					okdm = true
				}
			}
		}
	}
	if okdm && okdw && okmon {
		return true
	}
	return false
}

//RuName вернет имя сущности s на русском языке
func RuName(s string) string {
	switch s {
	case "contracts":
		return "Контракты"
	case "goods":
		return "Номенклатура"
	case "salesmatrix":
		return "Матрица товаров"
	case "orders":
		return "Заказы"
	case "contractgoods":
		return "Номенклатура поставщиков"
	case "stores":
		return "Склады"
	default:
		return s
	}
}
