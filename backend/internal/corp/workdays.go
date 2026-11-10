package corp

import "time"

const dateLayout = "2006-01-02"

// isWorkday — упрощённый производственный календарь: только выходные.
func isWorkday(d time.Time) bool {
	wd := d.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

// addWorkdays возвращает дату через n рабочих дней после d.
func addWorkdays(d time.Time, n int) time.Time {
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if isWorkday(d) {
			n--
		}
	}
	return d
}

// workdaysBetween считает рабочие дни в интервале [from, to] включительно.
func workdaysBetween(from, to time.Time) int {
	n := 0
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if isWorkday(d) {
			n++
		}
	}
	return n
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

var weekdaysRu = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

func humanDate(d time.Time) string {
	return d.Format("02.01") + " (" + weekdaysRu[d.Weekday()] + ")"
}
