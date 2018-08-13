package main

import (
	"fmt"
	"runtime"
	"time"
)

type Date struct {
	Day   int
	Month time.Month
	Year  int
}

func NewDate(t time.Time) *Date {
	d := new(Date)
	d.Year, d.Month, d.Day = t.Date()
	return d
}

func (d *Date) Set(s string) {
	t, err := time.ParseInLocation("Monday, January 2, 2006", s, timeZone)
	if err != nil {
		t, err = time.ParseInLocation("Mon 1/2", s, timeZone)
		if err != nil {
			fmt.Println(err.Error())
			stackStr := make([]byte, 1000, 1000)
			runtime.Stack(stackStr, false)
			fmt.Println(string(stackStr))
			return
		}
	}
	d.Year, d.Month, d.Day = t.Date()
}

func (d *Date) Today() {
	d.Year, d.Month, d.Day = time.Now().Date()
}

func (d *Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, timeZone)
}

func (d *Date) Format(layout string) string {
	return d.Time().Format(layout)
}

func (d *Date) AddDayTime(timeStr string) time.Time {
	t, err := time.Parse("3:04 PM", timeStr)
	if err != nil {
		t, err = time.Parse("3:04 PM MST", timeStr)
		if err != nil {
			fmt.Println(err.Error())
			stackStr := make([]byte, 1000, 1000)
			runtime.Stack(stackStr, false)
			fmt.Println(string(stackStr))
			return t
		}
	}

	midnight, _ := time.Parse("3:04 PM", "12:00 AM")
	dur := t.Sub(midnight)

	return d.Time().Add(dur)
}

/**********************************************************/

var timeZone *time.Location

/**********************************************************/

func init() {
	timeZone, _ = time.LoadLocation("America/New_York")
}
