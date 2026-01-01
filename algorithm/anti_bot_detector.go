package algorithm

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// UserBehavior 结构体定义了用户的单个行为事件。
// 它是机器人检测算法的基本输入。
type UserBehavior struct {
	UserID    uint64    // 用户的唯一标识符
	IP        string    // 用户的IP地址
	UserAgent string    // 用户的User-Agent字符串
	Timestamp time.Time // 行为发生的时间戳
	Action    string    // 用户的具体操作，例如 "view"（浏览），"add_to_cart"（加入购物车），"kill"（秒杀）
}

// AntiBotDetector 是一个防刷检测器，用于识别和评分潜在的机器人行为。
// 它通过分析用户的请求频率、行为模式和IP异常等多个维度进行判断。
type AntiBotDetector struct {
	// userRequests 存储每个用户ID在滑动窗口内的请求时间戳，用于检测用户级别的请求频率。
	userRequests map[uint64][]time.Time
	// ipRequests 存储每个IP地址在滑动窗口内的请求时间戳，用于检测IP级别的请求频率。
	ipRequests map[string][]time.Time

	// userBehaviors 存储每个用户ID的近期行为序列，用于分析行为模式。
	userBehaviors map[uint64][]UserBehavior

	mu sync.RWMutex // 读写锁，用于保护 detector 内部数据结构的并发访问
}

// NewAntiBotDetector 创建并返回一个新的 AntiBotDetector 实例。
// 它会初始化内部数据结构，并启动一个后台goroutine定期清理过期数据。
func NewAntiBotDetector() *AntiBotDetector {
	detector := &AntiBotDetector{
		userRequests:  make(map[uint64][]time.Time),
		ipRequests:    make(map[string][]time.Time),
		userBehaviors: make(map[uint64][]UserBehavior),
	}

	// 启动一个独立的goroutine定期清理过期的用户行为和请求记录。
	go detector.cleanup()
	slog.Info("AntiBotDetector initialized and background cleanup started")

	return detector
}

// IsBot 判断一个给定的用户行为是否属于机器人行为。
// 它综合了请求频率、行为模式和IP异常等多种检测维度。
// 返回值：一个布尔值表示是否是机器人，以及一个字符串说明判断为机器人的原因。
func (d *AntiBotDetector) IsBot(behavior UserBehavior) (bool, string) {
	start := time.Now()
	d.mu.Lock()         // 写锁，因为可能会记录新的行为。
	defer d.mu.Unlock() // 确保函数退出时解锁。

	// 1. 检查请求频率是否异常高。
	if isHighFrequency, reason := d.checkFrequency(behavior); isHighFrequency {
		slog.Warn("Bot detected by frequency", "user_id", behavior.UserID, "ip", behavior.IP, "reason", reason, "duration", time.Since(start))
		return true, reason
	}

	// 2. 检查用户行为模式是否存在可疑迹象。
	if isSuspicious, reason := d.checkBehaviorPattern(behavior); isSuspicious {
		slog.Warn("Bot detected by behavior pattern", "user_id", behavior.UserID, "ip", behavior.IP, "reason", reason, "duration", time.Since(start))
		return true, reason
	}

	// 3. 检查IP地址是否存在异常。
	if isAbnormalIP, reason := d.checkIPAbnormal(behavior); isAbnormalIP {
		slog.Warn("Bot detected by IP abnormal", "user_id", behavior.UserID, "ip", behavior.IP, "reason", reason, "duration", time.Since(start))
		return true, reason
	}

	// 如果未被判断为机器人，则记录当前行为，以供后续分析。
	d.recordBehavior(behavior)

	return false, "" // 不是机器人。
}

// checkFrequency 检查用户和IP的请求频率是否超过阈值。
// 使用滑动窗口机制来统计一段时间内的请求数量。
func (d *AntiBotDetector) checkFrequency(behavior UserBehavior) (bool, string) {
	now := behavior.Timestamp
	windowSize := 10 * time.Second // 定义滑动窗口大小为10秒。

	// 检查用户ID的请求频率：
	// 清理当前用户在滑动窗口之外的旧请求记录，只保留在窗口内的请求。
	if requests, exists := d.userRequests[behavior.UserID]; exists {
		validRequests := make([]time.Time, 0)
		for _, t := range requests {
			if now.Sub(t) <= windowSize {
				validRequests = append(validRequests, t)
			}
		}
		d.userRequests[behavior.UserID] = validRequests

		// 如果10秒内某个用户请求次数超过20次，则判断为高频率。
		if len(validRequests) >= 20 {
			return true, "用户请求频率过高"
		}
	}

	// 检查IP地址的请求频率：
	// 清理当前IP在滑动窗口之外的旧请求记录。
	if requests, exists := d.ipRequests[behavior.IP]; exists {
		validRequests := make([]time.Time, 0)
		for _, t := range requests {
			if now.Sub(t) <= windowSize {
				validRequests = append(validRequests, t)
			}
		}
		d.ipRequests[behavior.IP] = validRequests

		// 如果10秒内某个IP地址请求次数超过50次，则判断为高频率。
		// 注意：同一IP下可能存在多个正常用户，所以阈值通常高于单个用户的。
		if len(validRequests) >= 50 {
			return true, "IP请求频率过高"
		}
	}

	return false, ""
}

// checkBehaviorPattern 检查用户行为序列是否符合机器人特征。
// 例如，过于规律的时间间隔、缺少正常浏览行为直接进行秒杀等。
func (d *AntiBotDetector) checkBehaviorPattern(behavior UserBehavior) (bool, string) {
	behaviors, exists := d.userBehaviors[behavior.UserID]
	// 如果行为记录不足，或者用户不存在，则无法判断模式。
	if !exists || len(behaviors) < 5 { // 至少需要5个行为才能初步分析模式。
		return false, ""
	}

	// 提取用户最近的10个行为进行分析。
	recentBehaviors := behaviors
	if len(behaviors) > 10 {
		recentBehaviors = behaviors[len(behaviors)-10:]
	}

	// 1. 检查行为之间的时间间隔是否过于规律，这是机器人脚本的常见特征。
	if d.isRegularInterval(recentBehaviors) {
		return true, "请求时间间隔过于规律"
	}

	// 2. 检查是否存在直接进行秒杀（"kill"）而缺少之前的浏览（"view"）等正常操作。
	if d.isDirectKill(recentBehaviors, behavior) {
		return true, "缺少正常浏览行为"
	}

	// 3. 检查UserAgent字符串是否异常，例如是空白的、伪造的或者非常罕见的。
	if d.isAbnormalUserAgent(behavior.UserAgent) {
		return true, "UserAgent异常"
	}

	return false, ""
}

// isRegularInterval 检查给定行为序列的时间间隔是否过于规律。
// 通过计算行为间隔的方差来判断，方差越小表示越规律。
func (d *AntiBotDetector) isRegularInterval(behaviors []UserBehavior) bool {
	if len(behaviors) < 3 { // 至少需要3个行为才能计算2个间隔。
		return false
	}

	intervals := make([]float64, 0)
	for i := 1; i < len(behaviors); i++ {
		// 计算相邻行为之间的时间间隔（秒）。
		interval := behaviors[i].Timestamp.Sub(behaviors[i-1].Timestamp).Seconds()
		intervals = append(intervals, interval)
	}

	// 计算时间间隔的平均值。
	mean := 0.0
	for _, interval := range intervals {
		mean += interval
	}
	mean /= float64(len(intervals))

	// 计算时间间隔的方差。
	variance := 0.0
	for _, interval := range intervals {
		variance += (interval - mean) * (interval - mean)
	}
	variance /= float64(len(intervals))

	// 如果方差非常小（例如小于0.1）且平均间隔也很小（例如小于2秒），则认为时间间隔过于规律。
	return variance < 0.1 && mean < 2.0
}

// isDirectKill 检查当前行为是否是“直接秒杀”行为，即缺少之前的浏览行为。
// 这通常是机器人快速抢购的特征。
func (d *AntiBotDetector) isDirectKill(behaviors []UserBehavior, current UserBehavior) bool {
	if current.Action != "kill" { // 只对秒杀行为进行判断。
		return false
	}

	// 检查最近的5个行为中是否有“view”（浏览）行为。
	hasView := false
	for i := len(behaviors) - 1; i >= 0 && i >= len(behaviors)-5; i-- {
		if behaviors[i].Action == "view" {
			hasView = true
			break
		}
	}

	return !hasView // 如果没有浏览行为，则认为是直接秒杀。
}

// isAbnormalUserAgent 检查User-Agent字符串是否异常。
func (d *AntiBotDetector) isAbnormalUserAgent(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if ua == "" || len(ua) < 10 {
		return true // 空或过短的 UA 视为异常
	}

	// 1. 检查已知的爬虫标识
	botPatterns := []string{"bot", "crawler", "spider", "scrap", "curl", "wget", "python", "http-client"}
	for _, p := range botPatterns {
		if strings.Contains(ua, p) {
			return true
		}
	}

	// 2. 检查常见的正常浏览器 User-Agent 标识。
	normalAgents := []string{"mozilla", "chrome", "safari", "firefox", "edge", "applewebkit"}
	for _, agent := range normalAgents {
		if strings.Contains(ua, agent) {
			return false
		}
	}

	return true // 如果不包含任何正常标识且通过了爬虫检查，仍视为异常 (可能是罕见或自定义客户端)
}

// checkIPAbnormal 检查IP地址是否存在异常。
// 例如，一个IP地址下关联了过多的用户。
func (d *AntiBotDetector) checkIPAbnormal(behavior UserBehavior) (bool, string) {
	// 统计同一IP地址下活跃的用户数量。
	ipUsers := make(map[uint64]bool) // 使用map来去重用户ID。

	for userID, behaviors := range d.userBehaviors {
		for _, b := range behaviors {
			if b.IP == behavior.IP {
				ipUsers[userID] = true
			}
		}
	}

	// 如果同一IP下关联的用户数量超过10个，则认为IP异常。
	if len(ipUsers) > 10 {
		return true, "同一IP用户数过多"
	}

	return false, ""
}

// recordBehavior 记录用户的最新行为。
func (d *AntiBotDetector) recordBehavior(behavior UserBehavior) {
	// 记录用户请求时间。
	d.userRequests[behavior.UserID] = append(d.userRequests[behavior.UserID], behavior.Timestamp)

	// 记录IP请求时间。
	d.ipRequests[behavior.IP] = append(d.ipRequests[behavior.IP], behavior.Timestamp)

	// 记录用户行为序列。
	d.userBehaviors[behavior.UserID] = append(d.userBehaviors[behavior.UserID], behavior)

	// 限制每个用户的行为记录数量，防止内存无限增长。
	if len(d.userBehaviors[behavior.UserID]) > 100 { // 只保留最新的100个行为。
		d.userBehaviors[behavior.UserID] = d.userBehaviors[behavior.UserID][50:] // 保留最近50个行为，丢弃最旧的50个。
	}
}

// cleanup 定期清理过期数据。
// 此函数在一个独立的goroutine中运行，每分钟执行一次，清理超出保留时间的用户请求和行为记录。
func (d *AntiBotDetector) cleanup() {
	ticker := time.NewTicker(1 * time.Minute) // 每分钟触发一次。
	defer ticker.Stop()                       // 函数退出时停止ticker。

	for range ticker.C { // 循环等待ticker事件。
		start := time.Now()
		d.mu.Lock() // 加写锁，以安全地修改数据。

		now := time.Now()
		expireTime := 5 * time.Minute // 数据保留5分钟。
		userCount, ipCount, behaviorCount := 0, 0, 0

		// 清理用户请求记录。
		for userID, requests := range d.userRequests {
			validRequests := make([]time.Time, 0)
			for _, t := range requests {
				if now.Sub(t) <= expireTime {
					validRequests = append(validRequests, t)
				}
			}
			if len(validRequests) == 0 {
				delete(d.userRequests, userID) // 如果没有有效请求，则删除该用户的所有记录。
				userCount++
			} else {
				d.userRequests[userID] = validRequests
			}
		}

		// 清理IP请求记录。
		for ip, requests := range d.ipRequests {
			validRequests := make([]time.Time, 0)
			for _, t := range requests {
				if now.Sub(t) <= expireTime {
					validRequests = append(validRequests, t)
				}
			}
			if len(validRequests) == 0 {
				delete(d.ipRequests, ip)
				ipCount++
			} else {
				d.ipRequests[ip] = validRequests
			}
		}

		// 清理用户行为记录。
		for userID, behaviors := range d.userBehaviors {
			validBehaviors := make([]UserBehavior, 0)
			for _, b := range behaviors {
				if now.Sub(b.Timestamp) <= expireTime {
					validBehaviors = append(validBehaviors, b)
				}
			}
			if len(validBehaviors) == 0 {
				delete(d.userBehaviors, userID)
				behaviorCount++
			} else {
				d.userBehaviors[userID] = validBehaviors
			}
		}

		d.mu.Unlock() // 解锁。
		slog.Debug("AntiBotDetector cleanup finished", "deleted_users", userCount, "deleted_ips", ipCount, "deleted_behaviors", behaviorCount, "duration", time.Since(start))
	}
}

// GetRiskScore 根据用户的行为计算一个风险评分（0-100）。
// 分数越高表示机器人行为的可能性越大。
func (d *AntiBotDetector) GetRiskScore(behavior UserBehavior) int {
	d.mu.RLock()         // 读锁，因为只读取数据。
	defer d.mu.RUnlock() // 确保函数退出时解锁。

	score := 0 // 初始风险评分为0。

	// 1. 请求频率评分（权重较高，最高40分）
	if requests, exists := d.userRequests[behavior.UserID]; exists {
		windowSize := 10 * time.Second
		now := behavior.Timestamp

		count := 0
		for _, t := range requests {
			if now.Sub(t) <= windowSize {
				count++
			}
		}

		// 根据请求次数赋予不同分数。
		if count > 20 {
			score += 40
		} else if count > 10 {
			score += 20
		} else if count > 5 {
			score += 10
		}
	}

	// 2. 行为模式评分（次高权重，最高30分）
	if behaviors, exists := d.userBehaviors[behavior.UserID]; exists && len(behaviors) >= 3 {
		if d.isRegularInterval(behaviors) {
			score += 30 // 时间间隔过于规律，加高分。
		}
		if d.isDirectKill(behaviors, behavior) {
			score += 20 // 直接秒杀，加分。
		}
	}

	// 3. UserAgent评分（中等权重，最高20分）
	if d.isAbnormalUserAgent(behavior.UserAgent) {
		score += 20
	}

	// 4. IP评分（较低权重，最高10分）
	if isAbnormal, _ := d.checkIPAbnormal(behavior); isAbnormal {
		score += 10
	}

	// 确保总分不超过100。
	if score > 100 {
		score = 100
	}

	return score
}
