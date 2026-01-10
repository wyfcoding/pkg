package algorithm

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// UserBehavior 结构体定义了用户的单个行为事件.
type UserBehavior struct {
	Timestamp time.Time // 行为发生的时间
	IP        string    // 用户的 IP 地址
	UserAgent string    // 用户的 User-Agent 字符串
	Action    string    // 用户的具体操作
	UserID    uint64    // 用户的唯一标识
}

const (
	reasonHighFreq    = "high_request_frequency"
	reasonSuspicious  = "suspicious_pattern"
	reasonAbnormalIP  = "abnormal_ip_activity"
	reasonDirectKill  = "direct_kill_without_view"
	reasonRegularInt  = "regular_interval_detected"
	reasonBadUA       = "abnormal_user_agent"
	reasonManyUsersIP = "too_many_users_from_same_ip"
)

const (
	maxBehaviorHistory = 100
	keepBehaviorRecent = 50
	windowFreq         = 10 * time.Second
	expireData         = 5 * time.Minute
	userFreqLimit      = 20
	ipFreqLimit        = 50
	ipUserLimit        = 10
	uaMinLen           = 10
)

// AntiBotDetector 是一个防刷检测器.
type AntiBotDetector struct {
	userRequests  map[uint64][]time.Time
	ipRequests    map[string][]time.Time
	userBehaviors map[uint64][]UserBehavior
	mu            sync.RWMutex
}

// NewAntiBotDetector 创建并返回一个新的 AntiBotDetector 实例.
func NewAntiBotDetector() *AntiBotDetector {
	detector := &AntiBotDetector{
		userRequests:  make(map[uint64][]time.Time),
		ipRequests:    make(map[string][]time.Time),
		userBehaviors: make(map[uint64][]UserBehavior),
	}

	go detector.cleanupLoop()

	return detector
}

// IsBot 判断一个给定的用户行为是否属于机器人行为.
func (d *AntiBotDetector) IsBot(behavior *UserBehavior) (bool, string) {
	start := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	if isHigh, reason := d.checkFrequency(behavior); isHigh {
		slog.Warn("Bot detected by frequency", "user_id", behavior.UserID, "ip", behavior.IP, "reason", reason, "duration", time.Since(start))

		return true, reason
	}

	if isSuspicious, reason := d.checkBehaviorPattern(behavior); isSuspicious {
		slog.Warn("Bot detected by behavior pattern", "user_id", behavior.UserID, "ip", behavior.IP, "reason", reason, "duration", time.Since(start))

		return true, reason
	}

	if isAbnormal, reason := d.checkIPAbnormal(behavior); isAbnormal {
		slog.Warn("Bot detected by IP abnormal", "user_id", behavior.UserID, "ip", behavior.IP, "reason", reason, "duration", time.Since(start))

		return true, reason
	}

	d.recordBehavior(behavior)

	return false, ""
}

func (d *AntiBotDetector) checkFrequency(behavior *UserBehavior) (bool, string) {
	now := behavior.Timestamp

	if requests, exists := d.userRequests[behavior.UserID]; exists {
		valid := d.filterRequests(requests, now, windowFreq)
		d.userRequests[behavior.UserID] = valid
		if len(valid) >= userFreqLimit {
			return true, reasonHighFreq
		}
	}

	if requests, exists := d.ipRequests[behavior.IP]; exists {
		valid := d.filterRequests(requests, now, windowFreq)
		d.ipRequests[behavior.IP] = valid
		if len(valid) >= ipFreqLimit {
			return true, reasonHighFreq
		}
	}

	return false, ""
}

func (d *AntiBotDetector) filterRequests(requests []time.Time, now time.Time, window time.Duration) []time.Time {
	valid := make([]time.Time, 0, len(requests))
	for _, t := range requests {
		if now.Sub(t) <= window {
			valid = append(valid, t)
		}
	}

	return valid
}

func (d *AntiBotDetector) checkBehaviorPattern(behavior *UserBehavior) (bool, string) {
	behaviors, exists := d.userBehaviors[behavior.UserID]
	if !exists || len(behaviors) < 5 {
		return false, ""
	}

	recent := behaviors
	if len(behaviors) > 10 {
		recent = behaviors[len(behaviors)-10:]
	}

	if d.isRegularInterval(recent) {
		return true, reasonRegularInt
	}

	if d.isDirectKill(recent, behavior) {
		return true, reasonDirectKill
	}

	if d.isAbnormalUserAgent(behavior.UserAgent) {
		return true, reasonBadUA
	}

	return false, ""
}

func (d *AntiBotDetector) isRegularInterval(behaviors []UserBehavior) bool {
	if len(behaviors) < 3 {
		return false
	}

	intervals := make([]float64, 0, len(behaviors)-1)
	var sum float64
	for i := 1; i < len(behaviors); i++ {
		interval := behaviors[i].Timestamp.Sub(behaviors[i-1].Timestamp).Seconds()
		intervals = append(intervals, interval)
		sum += interval
	}

	mean := sum / float64(len(intervals))
	var varianceSum float64
	for _, interval := range intervals {
		diff := interval - mean
		varianceSum += diff * diff
	}
	variance := varianceSum / float64(len(intervals))

	return variance < 0.1 && mean < 2.0
}

func (d *AntiBotDetector) isDirectKill(behaviors []UserBehavior, current *UserBehavior) bool {
	if current.Action != "kill" {
		return false
	}

	hasView := false
	for i := len(behaviors) - 1; i >= 0 && i >= len(behaviors)-5; i-- {
		if behaviors[i].Action == "view" {
			hasView = true
			break
		}
	}

	return !hasView
}

func (d *AntiBotDetector) isAbnormalUserAgent(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if ua == "" || len(ua) < uaMinLen {
		return true
	}

	botPatterns := []string{"bot", "crawler", "spider", "scrap", "curl", "wget", "python", "http-client"}
	for _, p := range botPatterns {
		if strings.Contains(ua, p) {
			return true
		}
	}

	normalAgents := []string{"mozilla", "chrome", "safari", "firefox", "edge", "applewebkit"}
	for _, agent := range normalAgents {
		if strings.Contains(ua, agent) {
			return false
		}
	}

	return true
}

func (d *AntiBotDetector) checkIPAbnormal(behavior *UserBehavior) (bool, string) {
	ipUsers := make(map[uint64]bool)
	for userID, behaviors := range d.userBehaviors {
		for i := range behaviors {
			if behaviors[i].IP == behavior.IP {
				ipUsers[userID] = true
				break
			}
		}
	}

	if len(ipUsers) > ipUserLimit {
		return true, reasonManyUsersIP
	}

	return false, ""
}

func (d *AntiBotDetector) recordBehavior(behavior *UserBehavior) {
	d.userRequests[behavior.UserID] = append(d.userRequests[behavior.UserID], behavior.Timestamp)
	d.ipRequests[behavior.IP] = append(d.ipRequests[behavior.IP], behavior.Timestamp)
	d.userBehaviors[behavior.UserID] = append(d.userBehaviors[behavior.UserID], *behavior)

	if len(d.userBehaviors[behavior.UserID]) > maxBehaviorHistory {
		d.userBehaviors[behavior.UserID] = d.userBehaviors[behavior.UserID][len(d.userBehaviors[behavior.UserID])-keepBehaviorRecent:]
	}
}

func (d *AntiBotDetector) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.cleanup()
	}
}

func (d *AntiBotDetector) cleanup() {
	start := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var userCount, ipCount, behaviorCount int

	for userID, requests := range d.userRequests {
		valid := d.filterRequests(requests, now, expireData)
		if len(valid) == 0 {
			delete(d.userRequests, userID)
			userCount++
		} else {
			d.userRequests[userID] = valid
		}
	}

	for ip, requests := range d.ipRequests {
		valid := d.filterRequests(requests, now, expireData)
		if len(valid) == 0 {
			delete(d.ipRequests, ip)
			ipCount++
		} else {
			d.ipRequests[ip] = valid
		}
	}

	for userID, behaviors := range d.userBehaviors {
		valid := make([]UserBehavior, 0)
		for i := range behaviors {
			if now.Sub(behaviors[i].Timestamp) <= expireData {
				valid = append(valid, behaviors[i])
			}
		}
		if len(valid) == 0 {
			delete(d.userBehaviors, userID)
			behaviorCount++
		} else {
			d.userBehaviors[userID] = valid
		}
	}

	slog.Debug("AntiBotDetector cleanup finished", "deleted_users", userCount, "deleted_ips", ipCount, "deleted_behaviors", behaviorCount, "duration", time.Since(start))
}

// GetRiskScore 根据用户的行为计算一个风险评分.
func (d *AntiBotDetector) GetRiskScore(behavior *UserBehavior) int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	score := 0
	if requests, exists := d.userRequests[behavior.UserID]; exists {
		valid := d.filterRequests(requests, behavior.Timestamp, windowFreq)
		count := len(valid)
		switch {
		case count > 20:
			score += 40
		case count > 10:
			score += 20
		case count > 5:
			score += 10
		}
	}

	if behaviors, exists := d.userBehaviors[behavior.UserID]; exists && len(behaviors) >= 3 {
		if d.isRegularInterval(behaviors) {
			score += 30
		}
		if d.isDirectKill(behaviors, behavior) {
			score += 20
		}
	}

	if d.isAbnormalUserAgent(behavior.UserAgent) {
		score += 20
	}

	if isAbnormal, _ := d.checkIPAbnormal(behavior); isAbnormal {
		score += 10
	}

	if score > 100 {
		score = 100
	}

	return score
}
