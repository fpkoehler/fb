package main

import (
	"testing"
)

func TestDate(t *testing.T) {
	var d Date

	d.Set("Monday, March 20, 2017")
	t.Log("Date", d)
}
