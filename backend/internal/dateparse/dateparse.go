// Package dateparse извлекает даты и периоды из русскоязычного текста:
// «завтра», «в пятницу», «13 ноября», «13.11», «с 16 по 18 ноября», «на 2 дня».
package dateparse

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Range — период [From, To].
type Range struct {
	From, To time.Time
}

var months = map[string]time.Month{
	"январ": 1, "феврал": 2, "март": 3, "апрел": 4, "ма": 5, "июн": 6,
	"июл": 7, "август": 8, "сентябр": 9, "октябр": 10, "ноябр": 11, "декабр": 12,
}

var weekdays = []struct {
	stem string
	day  time.Weekday
}{
	{"понедельн", time.Monday}, {"вторн", time.Tuesday}, {"сред", time.Wednesday}, {"четверг", time.Thursday},
	{"пятниц", time.Friday}, {"суббот", time.Saturday}, {"воскресен", time.Sunday},
}

var (
	reISO       = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	reDotted    = regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})(?:\.(\d{2,4}))?\b`)
	reDayMonth  = regexp.MustCompile(`\b(\d{1,2})(?:\s*(?:-|–|—|по|и|до)\s*(\d{1,2}))?\s+(январ\S*|феврал\S*|март\S*|апрел\S*|ма[яй]\S*|июн\S*|июл\S*|август\S*|сентябр\S*|октябр\S*|ноябр\S*|декабр\S*)`)
	reWeekday   = regexp.MustCompile(`(понедельн|вторн|сред[уаы]|четверг|пятниц|суббот|воскресен)\S*`)
	reDuration  = regexp.MustCompile(`на\s+(\d+|два|три|пару)\s+(?:рабочих\s+)?д`)
	wordNumbers = map[string]int{"два": 2, "три": 3, "пару": 2}
)

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// date строит дату; если год не указан и дата прошла — берётся следующий год.
func date(now time.Time, year int, month time.Month, day int) (time.Time, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	explicit := year != 0
	if !explicit {
		year = now.Year()
	}
	d := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
	if d.Day() != day {
		return time.Time{}, false
	}
	if !explicit && d.Before(dayOf(now)) {
		d = d.AddDate(1, 0, 0)
	}
	return d, true
}

func nextWeekday(now time.Time, wd time.Weekday) time.Time {
	d := dayOf(now)
	for i := 1; i <= 7; i++ {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() == wd {
			return d
		}
	}
	return d
}

func weekdayOf(s string) time.Weekday {
	for _, w := range weekdays {
		if strings.HasPrefix(s, w.stem) || (w.stem == "сред" && strings.HasPrefix(s, "сред")) {
			return w.day
		}
	}
	return time.Monday
}

// Parse находит первую дату или период в тексте.
func Parse(text string, now time.Time) (Range, bool) {
	t := strings.ToLower(strings.ReplaceAll(text, "ё", "е"))
	today := dayOf(now)
	var r Range
	found := false
	switch {
	case reISO.MatchString(t):
		m := reISO.FindStringSubmatch(t)
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		r.From, found = date(now, y, time.Month(mo), d)
		r.To = r.From
	case reDayMonth.MatchString(t):
		m := reDayMonth.FindStringSubmatch(t)
		var mon time.Month
		for stem, v := range months {
			if strings.HasPrefix(m[3], stem) && (stem != "ма" || strings.HasPrefix(m[3], "мая") || strings.HasPrefix(m[3], "май")) {
				mon = v
			}
		}
		d1, _ := strconv.Atoi(m[1])
		r.From, found = date(now, 0, mon, d1)
		r.To = r.From
		if found && m[2] != "" {
			d2, _ := strconv.Atoi(m[2])
			if to, ok := date(now, r.From.Year(), mon, d2); ok && !to.Before(r.From) {
				r.To = to
			}
		}
	case reDotted.MatchString(t):
		m := reDotted.FindStringSubmatch(t)
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y := 0
		if m[3] != "" {
			y, _ = strconv.Atoi(m[3])
			if y < 100 {
				y += 2000
			}
		}
		r.From, found = date(now, y, time.Month(mo), d)
		r.To = r.From
	case strings.Contains(t, "послезавтра"):
		r.From, r.To, found = today.AddDate(0, 0, 2), today.AddDate(0, 0, 2), true
	case strings.Contains(t, "завтра"):
		r.From, r.To, found = today.AddDate(0, 0, 1), today.AddDate(0, 0, 1), true
	case strings.Contains(t, "сегодня"):
		r.From, r.To, found = today, today, true
	case reWeekday.MatchString(t):
		all := reWeekday.FindAllStringSubmatch(t, 2)
		r.From = nextWeekday(now, weekdayOf(all[0][1]))
		if strings.Contains(t, "следующ") && r.From.Sub(today) < 7*24*time.Hour && r.From.Weekday() > today.Weekday() {
			r.From = r.From.AddDate(0, 0, 7)
		}
		r.To, found = r.From, true
		if len(all) > 1 {
			to := r.From
			for to.Weekday() != weekdayOf(all[1][1]) {
				to = to.AddDate(0, 0, 1)
			}
			r.To = to
		}
	}
	if !found {
		return Range{}, false
	}
	if m := reDuration.FindStringSubmatch(t); m != nil && r.From.Equal(r.To) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = wordNumbers[m[1]]
		}
		to := r.From
		for n > 1 {
			to = to.AddDate(0, 0, 1)
			if to.Weekday() != time.Saturday && to.Weekday() != time.Sunday {
				n--
			}
		}
		r.To = to
	}
	return r, true
}
