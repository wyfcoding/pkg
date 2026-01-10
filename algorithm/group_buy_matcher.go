package algorithm

import (
	"slices"
	"time"
)

// MatchStrategy 定义了拼团匹配的策略类型.
type MatchStrategy int

const (
	// MatchStrategyFastest 最快成团.
	MatchStrategyFastest MatchStrategy = iota + 1
	// MatchStrategyNearest 最近地域.
	MatchStrategyNearest
	// MatchStrategyNewest 最新创建.
	MatchStrategyNewest
	// MatchStrategyAlmostFull 即将成团.
	MatchStrategyAlmostFull
)

const (
	regionScoreFull  = 1.0
	distanceScale    = 10000.0
	urgencyTimeScale = 60.0
	weightCompletion = 0.4
	weightUrgency    = 0.3
	weightRegion     = 0.3
)

// GroupBuyGroup 结构体定义了一个拼团活动组的详细信息.
type GroupBuyGroup struct {
	CreatedAt     time.Time
	ExpireAt      time.Time
	Region        string
	Lat           float64
	Lon           float64
	ID            uint64
	ActivityID    uint64
	LeaderID      uint64
	RequiredCount int
	CurrentCount  int
}

// GroupBuyMatcher 结构体提供了各种拼团组的匹配策略.
type GroupBuyMatcher struct{}

// NewGroupBuyMatcher 创建并返回一个新的 GroupBuyMatcher 实例.
func NewGroupBuyMatcher() *GroupBuyMatcher {
	return &GroupBuyMatcher{}
}

// FindBestGroup 根据指定的匹配策略，为用户找到一个最合适的拼团组.
func (m *GroupBuyMatcher) FindBestGroup(
	activityID uint64,
	userLat, userLon float64,
	userRegion string,
	groups []GroupBuyGroup,
	strategy MatchStrategy,
) *GroupBuyGroup {
	if len(groups) == 0 {
		return nil
	}

	available := make([]GroupBuyGroup, 0)
	now := time.Now()

	for i := range groups {
		g := &groups[i]
		if g.ActivityID == activityID &&
			g.CurrentCount < g.RequiredCount &&
			g.ExpireAt.After(now) {
			available = append(available, *g)
		}
	}

	if len(available) == 0 {
		return nil
	}

	switch strategy {
	case MatchStrategyFastest:
		return m.matchFastest(available)
	case MatchStrategyNearest:
		return m.matchNearest(available, userLat, userLon, userRegion)
	case MatchStrategyNewest:
		return m.matchNewest(available)
	case MatchStrategyAlmostFull:
		return m.matchAlmostFull(available)
	default:
		return m.matchFastest(available)
	}
}

func (m *GroupBuyMatcher) matchFastest(groups []GroupBuyGroup) *GroupBuyGroup {
	slices.SortFunc(groups, func(a, b GroupBuyGroup) int {
		remA := a.RequiredCount - a.CurrentCount
		remB := b.RequiredCount - b.CurrentCount

		if remA != remB {
			return remA - remB
		}

		if a.ExpireAt.After(b.ExpireAt) {
			return -1
		}

		if b.ExpireAt.After(a.ExpireAt) {
			return 1
		}

		return 0
	})

	return &groups[0]
}

func (m *GroupBuyMatcher) matchNearest(
	groups []GroupBuyGroup,
	userLat, userLon float64,
	userRegion string,
) *GroupBuyGroup {
	same := make([]GroupBuyGroup, 0)
	other := make([]GroupBuyGroup, 0)

	for i := range groups {
		if groups[i].Region == userRegion {
			same = append(same, groups[i])
		} else {
			other = append(other, groups[i])
		}
	}

	if len(same) > 0 {
		return m.sortByDistance(same, userLat, userLon)
	}

	if len(other) > 0 {
		return m.sortByDistance(other, userLat, userLon)
	}

	return nil
}

func (m *GroupBuyMatcher) sortByDistance(groups []GroupBuyGroup, lat, lon float64) *GroupBuyGroup {
	slices.SortFunc(groups, func(a, b GroupBuyGroup) int {
		distA := HaversineDistance(lat, lon, a.Lat, a.Lon)
		distB := HaversineDistance(lat, lon, b.Lat, b.Lon)
		if distA < distB {
			return -1
		}
		if distA > distB {
			return 1
		}

		return 0
	})

	return &groups[0]
}

func (m *GroupBuyMatcher) matchNewest(groups []GroupBuyGroup) *GroupBuyGroup {
	slices.SortFunc(groups, func(a, b GroupBuyGroup) int {
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}

		if b.CreatedAt.After(a.CreatedAt) {
			return 1
		}

		return 0
	})

	return &groups[0]
}

func (m *GroupBuyMatcher) matchAlmostFull(groups []GroupBuyGroup) *GroupBuyGroup {
	slices.SortFunc(groups, func(a, b GroupBuyGroup) int {
		rateA := float64(a.CurrentCount) / float64(a.RequiredCount)
		rateB := float64(b.CurrentCount) / float64(b.RequiredCount)
		if rateA > rateB {
			return -1
		}
		if rateA < rateB {
			return 1
		}

		return 0
	})

	return &groups[0]
}

// SmartMatch 实现“智能匹配”策略.
func (m *GroupBuyMatcher) SmartMatch(
	activityID uint64,
	userLat, userLon float64,
	userRegion string,
	groups []GroupBuyGroup,
) *GroupBuyGroup {
	if len(groups) == 0 {
		return nil
	}

	available := make([]GroupBuyGroup, 0)
	now := time.Now()

	for i := range groups {
		g := &groups[i]
		if g.ActivityID == activityID && g.CurrentCount < g.RequiredCount && g.ExpireAt.After(now) {
			available = append(available, *g)
		}
	}

	if len(available) == 0 {
		return nil
	}

	type groupScore struct {
		group *GroupBuyGroup
		score float64
	}

	scores := make([]groupScore, 0, len(available))
	for i := range available {
		g := &available[i]
		compRate := float64(g.CurrentCount) / float64(g.RequiredCount)
		remTime := g.ExpireAt.Sub(now).Minutes()
		urgency := 1.0 / (1.0 + remTime/urgencyTimeScale)

		var regScore float64
		if g.Region == userRegion {
			regScore = regionScoreFull
		} else {
			dist := HaversineDistance(userLat, userLon, g.Lat, g.Lon)
			regScore = 1.0 / (1.0 + dist/distanceScale)
		}

		total := weightCompletion*compRate + weightUrgency*urgency + weightRegion*regScore
		scores = append(scores, groupScore{group: g, score: total})
	}

	slices.SortFunc(scores, func(a, b groupScore) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}

		return 0
	})

	return scores[0].group
}

// BatchMatch 批量匹配功能.
func (m *GroupBuyMatcher) BatchMatch(
	activityID uint64,
	users []struct {
		Region string
		UserID uint64
		Lat    float64
		Lon    float64
	},
	groups []GroupBuyGroup,
) map[uint64]uint64 {
	result := make(map[uint64]uint64)
	avail := slices.Clone(groups)

	for _, u := range users {
		best := m.SmartMatch(activityID, u.Lat, u.Lon, u.Region, avail)
		if best != nil {
			result[u.UserID] = best.ID
			for i := range avail {
				if avail[i].ID == best.ID {
					avail[i].CurrentCount++

					break
				}
			}
		}
	}

	return result
}
