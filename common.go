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

func inChedule(ched string, t ...time.Time) bool {
	arr := strings.Fields(ched)
	if len(arr) < 3 {
		return false
	}
	now := time.Now()
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
		if strings.Contains(dw, ",") {
			if strings.Index(dw, strconv.Itoa(nowdw)) >= 0 { //dw = 0:7
				okdw = true
			}
		} else {
			if strings.Contains(dw, "-") {
				kof := 1
				if strings.Contains(dw, "/") {
					v, err := strconv.Atoi(dw[1+strings.Index(dw, "/"):])
					if err == nil {
						kof = v
					}
				}
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
				v, err := strconv.Atoi(dw)
				if err == nil && v == nowdw {
					okdw = true
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
