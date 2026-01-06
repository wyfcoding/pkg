package datetime

import "time"

// FormatTime 将时间格式化为标准字符串 "YYYY-MM-DD HH:MM:SS"。
// t: 待格式化的时间对象。
// 返回格式化后的时间字符串。
func FormatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// FormatDate 将时间格式化为标准日期字符串 "YYYY-MM-DD"。
// t: 待格式化的时间对象。
// 返回格式化后的日期字符串。
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// ParseTime 解析一个形如 "YYYY-MM-DD HH:MM:SS" 的时间字符串。
// s: 待解析的时间字符串。
// 返回解析后的时间对象和可能发生的错误。
func ParseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", s)
}

// ParseDate 解析一个形如 "YYYY-MM-DD" 的日期字符串。
// s: 待解析的日期字符串。
// 返回解析后的时间对象（时、分、秒、纳秒为零）和可能发生的错误。
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// StartOfDay 获取给定时间 t 所在天的开始时间（即当天00:00:00）。
// t: 任意时间点。
// 返回当天00:00:00的时间对象。
func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EndOfDay 获取给定时间 t 所在天的结束时间（即当天23:59:59.999999999）。
// t: 任意时间点。
// 返回当天23:59:59.999999999的时间对象。
func EndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

var (
	// customHolidays 维护了特定年份的法定节假日列表 (格式: YYYY-MM-DD)。
	// 包含调休后的长假，用于金融清算或营销活动的日期判定。
	customHolidays = map[string]bool{
		"2024-01-01": true, "2024-02-10": true, "2024-05-01": true,
	}
	// customWorkdays 维护了因节假日调休而需要补班的特殊工作日。
	// 虽然这些日期可能是周末，但在业务逻辑上应被视为交易日或普通工作日。
	customWorkdays = map[string]bool{
		"2024-02-04": true, "2024-02-18": true,
	}
)

// IsHoliday 判断给定日期是否为节假日。
// 升级实现：综合考虑法定节假日配置与补班调休逻辑。
func IsHoliday(t time.Time) bool {
	dateStr := t.Format("2006-01-02")

	// 1. 检查是否为显式配置的节假日
	if customHolidays[dateStr] {
		return true
	}

	// 2. 检查是否为显式配置的补班工作日
	if customWorkdays[dateStr] {
		return false
	}

	// 3. 默认周末判定
	weekday := t.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}
