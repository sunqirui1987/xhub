package catalog

import (
	"testing"
	"time"
)

// loc is Asia/Shanghai loaded once for all subtests.
var loc = func() *time.Location {
	l, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		l = time.FixedZone("CST", 8*3600)
	}
	return l
}()

func cst(year, month, day, hour, minute int) time.Time {
	return time.Date(year, time.Month(month), day, hour, minute, 0, 0, loc)
}

// TestIsPeakHour_WeekdayMorning: Mon 10:00 is peak.
func TestIsPeakHour_WeekdayMorning(t *testing.T) {
	// 2026-03-02 is a Monday (regular workday, not a holiday).
	if !IsPeakHour(cst(2026, 3, 2, 10, 0)) {
		t.Fatal("weekday 10:00 should be peak")
	}
}

// TestIsPeakHour_WeekdayAfternoon: Mon 15:00 is peak.
func TestIsPeakHour_WeekdayAfternoon(t *testing.T) {
	if !IsPeakHour(cst(2026, 3, 2, 15, 0)) {
		t.Fatal("weekday 15:00 should be peak")
	}
}

// TestIsPeakHour_WeekdaySaturdayIsOffpeak: normal Saturday 10:00 is off-peak.
func TestIsPeakHour_WeekdaySaturdayIsOffpeak(t *testing.T) {
	// 2026-03-07 is a Saturday with no 调休 entry.
	if IsPeakHour(cst(2026, 3, 7, 10, 0)) {
		t.Fatal("regular Saturday 10:00 should be off-peak")
	}
}

// TestIsPeakHour_Noon: 12:00 exactly is the boundary and should be off-peak.
func TestIsPeakHour_Noon(t *testing.T) {
	if IsPeakHour(cst(2026, 3, 2, 12, 0)) {
		t.Fatal("12:00 is outside the morning session [09:00,12:00) and should be off-peak")
	}
}

// TestIsPeakHour_MorningStart: 09:00 is the start of the morning session.
func TestIsPeakHour_MorningStart(t *testing.T) {
	if !IsPeakHour(cst(2026, 3, 2, 9, 0)) {
		t.Fatal("09:00 is inside [09:00,12:00) and should be peak")
	}
}

// TestIsPeakHour_AfternoonEnd: 18:00 exactly is off-peak.
func TestIsPeakHour_AfternoonEnd(t *testing.T) {
	if IsPeakHour(cst(2026, 3, 2, 18, 0)) {
		t.Fatal("18:00 is outside [14:00,18:00) and should be off-peak")
	}
}

// TestIsPeakHour_LunchBreak: 13:00 between the two sessions is off-peak.
func TestIsPeakHour_LunchBreak(t *testing.T) {
	if IsPeakHour(cst(2026, 3, 2, 13, 0)) {
		t.Fatal("13:00 is the lunch gap and should be off-peak")
	}
}

// TestIsPeakHour_PublicHoliday: 2026-01-01 (元旦) 10:00 is off-peak.
func TestIsPeakHour_PublicHoliday(t *testing.T) {
	if IsPeakHour(cst(2026, 1, 1, 10, 0)) {
		t.Fatal("public holiday (元旦) 10:00 should be off-peak")
	}
}

// TestIsPeakHour_HolidayWeekday: 2026-02-17 is a Tuesday during Spring Festival — off-peak.
func TestIsPeakHour_HolidayWeekday(t *testing.T) {
	if IsPeakHour(cst(2026, 2, 17, 10, 0)) {
		t.Fatal("春节 weekday 10:00 should be off-peak (holiday)")
	}
}

// TestIsPeakHour_MakeUpWorkSaturday: 2026-01-04 is a Sunday 调休补班 — peak.
func TestIsPeakHour_MakeUpWorkSaturday(t *testing.T) {
	// 2026-01-04 is Sunday but isOffDay=false in the holiday table (make-up day).
	if !IsPeakHour(cst(2026, 1, 4, 10, 0)) {
		t.Fatal("make-up work day (调休补班) 10:00 should be peak")
	}
}

// TestIsPeakHour_SpringFestivalMakeUp: 2026-02-14 is a Saturday 调休补班 — peak.
func TestIsPeakHour_SpringFestivalMakeUp(t *testing.T) {
	if !IsPeakHour(cst(2026, 2, 14, 10, 0)) {
		t.Fatal("春节 make-up Saturday 10:00 should be peak")
	}
}

// TestIsPeakHour_NationalDayHoliday: 2026-10-07 (Wednesday, holiday) is off-peak.
func TestIsPeakHour_NationalDayHoliday(t *testing.T) {
	if IsPeakHour(cst(2026, 10, 7, 10, 0)) {
		t.Fatal("国庆节 Wednesday 10:00 should be off-peak")
	}
}

// TestIsPeakHour_NationalDayMakeUp: 2026-09-20 is Sunday 调休补班 for 国庆节 — peak.
func TestIsPeakHour_NationalDayMakeUp(t *testing.T) {
	if !IsPeakHour(cst(2026, 9, 20, 10, 0)) {
		t.Fatal("国庆节 make-up Sunday 10:00 should be peak")
	}
}

// TestParseHolidays verifies the embedded JSON produces the right exception sets.
func TestParseHolidays(t *testing.T) {
	idx := parseHolidays(cnHolidaysJSON)
	if _, ok := idx.offDays["2026-01-01"]; !ok {
		t.Fatal("2026-01-01 should be an off-day")
	}
	if _, ok := idx.workDays["2026-01-04"]; !ok {
		t.Fatal("2026-01-04 should be a forced work-day")
	}
	if _, ok := idx.offDays["2026-03-02"]; ok {
		t.Fatal("2026-03-02 is a regular Monday; it should not be in offDays")
	}
}
