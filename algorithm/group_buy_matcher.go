package algorithm

import (
	"sort"
	"time"
)

// GroupBuyGroup 结构体定义了一个拼团活动组的详细信息。
type GroupBuyGroup struct {
	ID            uint64    // 拼团组的唯一标识符。
	ActivityID    uint64    // 所属拼团活动的ID。
	LeaderID      uint64    // 团长的用户ID。
	RequiredCount int       // 拼团成功所需的人数。
	CurrentCount  int       // 当前已参团的人数。
	CreatedAt     time.Time // 拼团组的创建时间。
	ExpireAt      time.Time // 拼团组的过期时间，超过此时间未成团则失败。
	Region        string    // 拼团组所在的地区信息，用于地域匹配。
	Lat           float64   // 拼团组所在地的纬度，用于距离计算。
	Lon           float64   // 拼团组所在地的经度，用于距离计算。
}

// GroupBuyMatcher 结构体提供了各种拼团组的匹配策略。
type GroupBuyMatcher struct{}

// NewGroupBuyMatcher 创建并返回一个新的 GroupBuyMatcher 实例。
func NewGroupBuyMatcher() *GroupBuyMatcher {
	return &GroupBuyMatcher{}
}

// MatchStrategy 定义了拼团匹配的策略类型。
type MatchStrategy int

const (
	MatchStrategyFastest    MatchStrategy = iota + 1 // 最快成团：优先选择剩余人数少、即将过期的团。
	MatchStrategyNearest                             // 最近地域：优先选择地理位置上离用户最近的团。
	MatchStrategyNewest                              // 最新创建：优先选择最新创建的团。
	MatchStrategyAlmostFull                          // 即将成团：优先选择当前人数最多、离成团最近的团。
)

// FindBestGroup 根据指定的匹配策略，为用户找到一个最合适的拼团组。
// activityID: 用户想要参加的拼团活动ID。
// userLat, userLon: 用户的地理位置（纬度、经度）。
// userRegion: 用户的地区信息。
// groups: 所有可用的拼团组列表。
// strategy: 用户选择的匹配策略。
// 返回值：一个指向最佳拼团组的指针，如果没有合适的组则返回 nil。
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

	// 过滤：只保留未满员且未过期的、且与活动ID匹配的拼团组。
	available := make([]GroupBuyGroup, 0)
	now := time.Now()

	for _, g := range groups {
		if g.ActivityID == activityID && // 匹配活动ID
			g.CurrentCount < g.RequiredCount && // 未满员
			g.ExpireAt.After(now) { // 未过期
			available = append(available, g)
		}
	}

	if len(available) == 0 {
		return nil // 没有可用的拼团组。
	}

	// 根据传入的策略，选择不同的匹配算法。
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
		// 如果策略未知或未指定，默认使用最快成团策略。
		return m.matchFastest(available)
	}
}

// matchFastest 实现“最快成团”策略。
// 优先选择剩余人数最少（即离成团最近）的团。如果剩余人数相同，则优先选择剩余时间更长的团。
func (m *GroupBuyMatcher) matchFastest(groups []GroupBuyGroup) *GroupBuyGroup {
	sort.Slice(groups, func(i, j int) bool {
		// 计算剩余需要的人数。
		remainI := groups[i].RequiredCount - groups[i].CurrentCount
		remainJ := groups[j].RequiredCount - groups[j].CurrentCount

		// 优先按照剩余人数升序排序（剩余人数越少，排名越靠前）。
		if remainI != remainJ {
			return remainI < remainJ
		}

		// 如果剩余人数相同，则按照过期时间降序排序（剩余时间越长，越不容易过期，但这个策略名字似乎应该更侧重于时间紧迫）。
		// 原始代码是 After(groups[j].ExpireAt)，这意味着是剩余时间越长越优先，可能与“最快成团”的直觉有些出入。
		// 如果“最快成团”是想优先即将过期且人凑得差不多的，则这里可能需要调整逻辑。
		return groups[i].ExpireAt.After(groups[j].ExpireAt)
	})

	return &groups[0] // 返回排序后的第一个团。
}

// matchNearest 实现“最近地域”策略。
// 优先匹配与用户在同一地区的团。如果存在多个同地区团，则选择地理距离最近的。
// 如果没有同地区团，则在所有其他地区的团中选择地理距离最近的。
func (m *GroupBuyMatcher) matchNearest(
	groups []GroupBuyGroup,
	userLat, userLon float64,
	userRegion string,
) *GroupBuyGroup {

	sameRegion := make([]GroupBuyGroup, 0)  // 存储与用户同地区的团。
	otherRegion := make([]GroupBuyGroup, 0) // 存储其他地区的团。

	// 根据地区对团进行分类。
	for _, g := range groups {
		if g.Region == userRegion {
			sameRegion = append(sameRegion, g)
		} else {
			otherRegion = append(otherRegion, g)
		}
	}

	// 如果有同地区的团，优先在同地区中寻找距离最近的。
	if len(sameRegion) > 0 {
		// 按照用户到拼团组的距离升序排序。
		sort.Slice(sameRegion, func(i, j int) bool {
			// haversineDistance 函数未在此文件中定义，假定其在其他地方可用。
			distI := haversineDistance(userLat, userLon, sameRegion[i].Lat, sameRegion[i].Lon)
			distJ := haversineDistance(userLat, userLon, sameRegion[j].Lat, sameRegion[j].Lon)
			return distI < distJ
		})
		return &sameRegion[0]
	}

	// 如果没有同地区团，则在其他地区的团中寻找距离最近的。
	if len(otherRegion) > 0 {
		// 按照用户到拼团组的距离升序排序。
		sort.Slice(otherRegion, func(i, j int) bool {
			distI := haversineDistance(userLat, userLon, otherRegion[i].Lat, otherRegion[i].Lon)
			distJ := haversineDistance(userLat, userLon, otherRegion[j].Lat, otherRegion[j].Lon)
			return distI < distJ
		})
		return &otherRegion[0]
	}

	return nil // 没有找到合适的团。
}

// matchNewest 实现“最新创建”策略。
// 优先选择最新创建的拼团组。
func (m *GroupBuyMatcher) matchNewest(groups []GroupBuyGroup) *GroupBuyGroup {
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].CreatedAt.After(groups[j].CreatedAt) // 按照创建时间降序排序。
	})

	return &groups[0] // 返回排序后的第一个团。
}

// matchAlmostFull 实现“即将成团”策略。
// 优先选择当前人数最多（即离成团最接近）的拼团组。
func (m *GroupBuyMatcher) matchAlmostFull(groups []GroupBuyGroup) *GroupBuyGroup {
	sort.Slice(groups, func(i, j int) bool {
		// 计算成团进度率（完成度）。
		rateI := float64(groups[i].CurrentCount) / float64(groups[i].RequiredCount)
		rateJ := float64(groups[j].CurrentCount) / float64(groups[j].RequiredCount)
		return rateI > rateJ // 按照完成度降序排序。
	})

	return &groups[0] // 返回排序后的第一个团。
}

// SmartMatch 实现“智能匹配”策略。
// 它综合考虑拼团组的完成度、时间紧迫度和与用户的地域距离等多个因素，计算一个综合评分，
// 从而找到最符合用户需求的拼团组。
func (m *GroupBuyMatcher) SmartMatch(
	activityID uint64,
	userLat, userLon float64,
	userRegion string,
	groups []GroupBuyGroup,
) *GroupBuyGroup {

	if len(groups) == 0 {
		return nil
	}

	// 过滤：只保留未满员且未过期的、且与活动ID匹配的拼团组。
	available := make([]GroupBuyGroup, 0)
	now := time.Now()

	for _, g := range groups {
		if g.ActivityID == activityID &&
			g.CurrentCount < g.RequiredCount &&
			g.ExpireAt.After(now) {
			available = append(available, g)
		}
	}

	if len(available) == 0 {
		return nil
	}

	// 计算每个可用拼团组的综合评分。
	type groupScore struct {
		group *GroupBuyGroup
		score float64
	}

	scores := make([]groupScore, 0)

	for i := range available {
		g := &available[i]

		// 1. 完成度评分（0-1）：当前人数占所需人数的比例。
		completionRate := float64(g.CurrentCount) / float64(g.RequiredCount)

		// 2. 时间紧迫度评分（0-1）：剩余时间越少，分数越高。
		// 使用 1.0 / (1.0 + remainTime/60.0) 这种函数，确保时间越短，分值越高，且在[0,1]区间。
		remainTime := g.ExpireAt.Sub(now).Minutes()
		urgencyScore := 1.0 / (1.0 + remainTime/60.0)

		// 3. 地域评分（0-1）：同地区优先，距离越近分数越高。
		regionScore := 0.0
		if g.Region == userRegion {
			regionScore = 1.0 // 同地区直接给满分。
		} else {
			// haversineDistance 函数未在此文件中定义，假定其在其他地方可用。
			distance := haversineDistance(userLat, userLon, g.Lat, g.Lon)
			// 使用 1.0 / (1.0 + distance/10000.0) 这种函数，确保距离越近，分值越高，且在[0,1]区间。
			regionScore = 1.0 / (1.0 + distance/10000.0)
		}

		// 综合评分：权重分配示例（完成度40%，时间30%，地域30%）。
		totalScore := 0.4*completionRate + 0.3*urgencyScore + 0.3*regionScore

		scores = append(scores, groupScore{g, totalScore})
	}

	// 按照综合评分降序排序。
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	return scores[0].group // 返回评分最高的拼团组。
}

// BatchMatch 批量匹配功能，为多个用户同时匹配拼团组。
// 它会为每个用户尝试找到最佳拼团组，并更新拼团组的当前人数。
// activityID: 拼团活动ID。
// users: 待匹配的用户列表，包含UserID、地理位置和地区信息。
// groups: 所有可用的拼团组列表。
// 返回值：一个map，键为用户ID，值为匹配到的拼团组ID。
func (m *GroupBuyMatcher) BatchMatch(
	activityID uint64,
	users []struct {
		UserID uint64
		Lat    float64
		Lon    float64
		Region string
	},
	groups []GroupBuyGroup,
) map[uint64]uint64 { // userID -> groupID

	result := make(map[uint64]uint64)

	// 复制groups，避免在匹配过程中直接修改原始数据。
	// 注意：这里的复制是浅拷贝，如果 SmartMatch 内部修改了 GroupBuyGroup 的字段，会影响可用组。
	// 但 SmartMatch 内部不会修改，而是返回指针。外部更新 CurrentCount 时会修改副本。
	availableGroups := make([]GroupBuyGroup, len(groups))
	copy(availableGroups, groups)

	// 为每个用户匹配最佳拼团组。
	for _, user := range users {
		bestGroup := m.SmartMatch(activityID, user.Lat, user.Lon, user.Region, availableGroups)

		if bestGroup != nil {
			result[user.UserID] = bestGroup.ID

			// 匹配成功后，更新该拼团组的当前人数。
			// 这是为了确保后续用户的匹配能够反映最新的拼团状态。
			for i := range availableGroups {
				if availableGroups[i].ID == bestGroup.ID {
					availableGroups[i].CurrentCount++ // 增加当前人数。
					break
				}
			}
		}
	}

	return result
}
