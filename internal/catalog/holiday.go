package catalog

import (
	_ "embed"
	"encoding/json"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

//go:embed publicdata/cn_holidays.json
var cnHolidaysJSON []byte

// holidayDay is one entry in the holiday-cn JSON table.
// IsOffDay=true means this date is a rest day (public holiday or a regular
// weekday moved to a long weekend). IsOffDay=false means this date is a work
// day even though it falls on a weekend (调休补班).
type holidayDay struct {
	Name     string `json:"name"`
	Date     string `json:"date"` // YYYY-MM-DD
	IsOffDay bool   `json:"isOffDay"`
}

// holidayYear is one year's worth of entries in the holiday-cn format.
type holidayYear struct {
	Year   int          `json:"year"`
	Source string       `json:"source"`
	Days   []holidayDay `json:"days"`
}

// holidayIndex holds the two exception sets derived from the holiday table.
type holidayIndex struct {
	// offDays are dates that are explicitly rest days regardless of weekday.
	offDays map[string]struct{}
	// workDays are dates that are explicitly work days regardless of being weekend.
	workDays map[string]struct{}
}

var (
	shanghaiLoc    *time.Location
	loadedHolidays holidayIndex
)

// init 载入计费时段要用的两份数据：北京时间时区和中国法定节假日表。
//
// 两者都在进程启动时定好，之后只读。时区不能用进程时区：部署在别的时区会让
// 高峰判定整体偏一班，而高峰价是空闲价的两倍。
//
// 参数：无。
// 返回：无。结果写进本文件的 shanghaiLoc 和 loadedHolidays。
// 调用：Go 在载入这个包时自动执行。
// 测试：holiday_test.go 覆盖时段判定；这里的两份数据也可以被直接断言。
func init() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback: fixed UTC+8, no DST. China has not observed DST since 1991.
		logx.Error("Asia/Shanghai is unavailable; peak-hour billing falls back to a fixed UTC+8 offset err=%v", err)
		loc = time.FixedZone("CST", 8*3600)
	}
	shanghaiLoc = loc
	loadedHolidays = parseHolidays(cnHolidaysJSON)
	if len(loadedHolidays.offDays) == 0 && len(loadedHolidays.workDays) == 0 {
		// An unreadable table is not fatal, but it changes what customers are
		// charged: every weekday 09:00-12:00 and 14:00-18:00 becomes peak,
		// including public holidays, which the published rates exclude.
		logx.Error("cn_holidays is unreadable; peak hours will be judged on weekdays alone")
	}
}

// parseHolidays parses the holiday-cn JSON array into a look-up index.
// 参数 data（[]byte）：cn_holidays.json 的字节。
// 返回 holidayIndex（holidayIndex）：两个集合：强制休息日和强制工作日。解析失败时两个集合都空。
// 调用：init。
// 测试：holiday_test.go
func parseHolidays(data []byte) holidayIndex {
	idx := holidayIndex{
		offDays:  make(map[string]struct{}),
		workDays: make(map[string]struct{}),
	}
	var years []holidayYear
	if err := json.Unmarshal(data, &years); err != nil {
		logx.Error("cn_holidays is not readable err=%v", err)
		return idx
	}
	for _, y := range years {
		for _, d := range y.Days {
			if d.IsOffDay {
				idx.offDays[d.Date] = struct{}{}
			} else {
				idx.workDays[d.Date] = struct{}{}
			}
		}
	}
	return idx
}

// IsPeakHour reports whether t falls inside a peak billing window.
//
// Peak is defined as Mon–Fri 09:00–12:00 and 14:00–18:00 Beijing time
// (Asia/Shanghai), excluding Chinese public holidays and including make-up
// work days that fall on a weekend (调休补班). The rule mirrors DeepSeek's
// published pricing; any other provider with a window-aware rate table uses
// the same function because the window dimension is a property of the rate,
// not of the provider.
//
// Boundary treatment: 09:00 and 14:00 are peak; 12:00 and 18:00 are not.
// This matches the convention of half-open intervals [start, end).
//
// 参数 t（time.Time）：这次调用的开始时刻，精确到秒即可。
// 返回 bool（bool）：在高峰时段内时为真。
// 调用：计费路径 spend.go（尚未实现）。
// 测试：holiday_test.go
func IsPeakHour(t time.Time) bool {
	t = t.In(shanghaiLoc)
	date := t.Format("2006-01-02")

	// Explicit override: forced rest day (public holiday or adjusted weekend).
	if _, off := loadedHolidays.offDays[date]; off {
		return false
	}

	// Explicit override: forced work day (make-up weekend, 调休补班).
	_, forceWork := loadedHolidays.workDays[date]

	wd := t.Weekday()
	isWeekend := wd == time.Saturday || wd == time.Sunday
	isWorkDay := !isWeekend || forceWork

	if !isWorkDay {
		return false
	}

	h, m, _ := t.Clock()
	mins := h*60 + m
	// [09:00, 12:00) = [540, 720)   morning session
	// [14:00, 18:00) = [840, 1080)  afternoon session
	return (mins >= 540 && mins < 720) || (mins >= 840 && mins < 1080)
}
