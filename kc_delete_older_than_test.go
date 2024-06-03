package main

import (
	"testing"
	"time"
)

func TestDaysSinceCreation(t *testing.T) {
	now := time.Now()
	epochMillis := now.UnixNano() / int64(time.Millisecond)
	// Subtract 5 days worth of milliseconds
	createdAt := epochMillis - (5 * 24 * 60 * 60 * 1000)
	days := daysSinceCreation(createdAt)
	if days != 5 {
		t.Errorf("Expected 5, but got %d", days)
	}
}

func TestDaysToEpoch(t *testing.T) {
	now := time.Now()
	epochMillis := now.UnixNano() / int64(time.Millisecond)
	daysAgoEpochMillis := daysToEpoch(5)
	if epochMillis-daysAgoEpochMillis < (5*24*60*60*1000)-1000 || epochMillis-daysAgoEpochMillis > (5*24*60*60*1000)+1000 {
		t.Errorf("Expected approximately %d, but got %d", 5*24*60*60*1000, epochMillis-daysAgoEpochMillis)
	}
}

func TestSubtractDaysToDate(t *testing.T) {

	date, err := time.Parse(DateFormat, "2022-01-01")

	if err != nil {
		t.Errorf("Can't parse test date %q", err)
	}

	got := subtractDaysToDate(4, date)
	want := "2021-12-28"

	if got != want {
		t.Errorf("got %q, wanted %q", got, want)
	}
}

func TestParseDateToEpoch(t *testing.T) {

	// 2022-01-01 00:00:00 +0000 UTC = 1640995200000

	got, err := parseDateToEpoch("2022-01-01")
	if err != nil {
		t.Errorf("Can't parse test date %q", err)
	}
	want := int64(1640995200000)

	if got != want {
		t.Errorf("got %d, wanted %d", got, want)
	}
}

func TestEpochToDateString(t *testing.T) {

	got := epochToDateString(1640995200000)
	want := "2022-01-01"

	if got != want {
		t.Errorf("got %q, wanted %q", got, want)
	}
}

func TestParseDate(t *testing.T) {
	dateString := "2023-07-01"
	expectedDate, _ := time.Parse(DateFormat, dateString)
	actualDate, err := parseDate(dateString)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if actualDate != expectedDate {
		t.Errorf("Expected %v, but got %v", expectedDate, actualDate)
	}
}
