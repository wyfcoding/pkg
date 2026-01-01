package utils

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

// IsHoliday 判断给定日期是否为节假日（简化版，仅包含周末和固定日期的主要节假日）。
func IsHoliday(t time.Time) bool {
	// 1. 周末判断
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return true
	}

	// 2. 固定日期节假日判断 (示例：元旦、劳动节、国庆节)
	month := t.Month()
	day := t.Day()

	if month == time.January && day == 1 { // 元旦
		return true
	}
	if month == time.May && day == 1 { // 劳动节
		return true
	}
	if month == time.October && (day >= 1 && day <= 3) { // 国庆节 (前3天)
		return true
	}

	// 注意：由于农历节假日每年变动，建议生产环境对接专门的日历服务。
	return false
}
