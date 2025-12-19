package algorithm

import (
	"math"
	"sort"
	"sync"
	"time"
)

// RecommendationEngine 结构体实现了多种推荐算法。
// 它维护用户-物品交互数据，并基于这些数据生成推荐列表。
type RecommendationEngine struct {
	userItemMatrix map[uint64]map[uint64]float64 // 用户-物品评分矩阵 (UserID -> ItemID -> Rating)。
	itemUserMatrix map[uint64]map[uint64]float64 // 物品-用户评分矩阵 (ItemID -> UserID -> Rating)。
	itemViews      map[uint64]int                // 物品的浏览次数 (ItemID -> Count)。
	itemSales      map[uint64]int                // 物品的销售数量 (ItemID -> Count)。
	itemScores     map[uint64]float64            // 物品的综合得分，用于热门推荐 (ItemID -> Score)。
	lastUpdate     map[uint64]time.Time          // 物品最后一次更新时间，用于时间衰减 (ItemID -> Time)。
	mu             sync.RWMutex                  // 读写锁，用于保护引擎内部数据结构的并发访问。
}

// NewRecommendationEngine 创建并返回一个新的 RecommendationEngine 实例。
func NewRecommendationEngine() *RecommendationEngine {
	return &RecommendationEngine{
		userItemMatrix: make(map[uint64]map[uint64]float64),
		itemUserMatrix: make(map[uint64]map[uint64]float64),
		itemViews:      make(map[uint64]int),
		itemSales:      make(map[uint64]int),
		itemScores:     make(map[uint64]float64),
		lastUpdate:     make(map[uint64]time.Time),
	}
}

// AddRating 添加用户对物品的评分或隐式反馈（如浏览、收藏等）。
// userID: 用户的唯一标识符。
// itemID: 物品的唯一标识符。
// rating: 评分值，通常用于显式反馈（如1-5星），也可用于隐式反馈（如浏览记为1.0）。
func (re *RecommendationEngine) AddRating(userID, itemID uint64, rating float64) {
	re.mu.Lock()         // 加写锁。
	defer re.mu.Unlock() // 确保函数退出时解锁。

	if re.userItemMatrix[userID] == nil {
		re.userItemMatrix[userID] = make(map[uint64]float64)
	}
	re.userItemMatrix[userID][itemID] = rating // 更新用户-物品矩阵。

	if re.itemUserMatrix[itemID] == nil {
		re.itemUserMatrix[itemID] = make(map[uint64]float64)
	}
	re.itemUserMatrix[itemID][userID] = rating // 更新物品-用户矩阵。

	re.lastUpdate[itemID] = time.Now() // 更新物品的最后活动时间。
}

// AddView 添加物品的浏览记录。
func (re *RecommendationEngine) AddView(itemID uint64) {
	re.mu.Lock()
	defer re.mu.Unlock()

	re.itemViews[itemID]++             // 增加物品的浏览次数。
	re.lastUpdate[itemID] = time.Now() // 更新物品的最后活动时间。
}

// AddSale 添加物品的销售记录。
func (re *RecommendationEngine) AddSale(itemID uint64) {
	re.mu.Lock()
	defer re.mu.Unlock()

	re.itemSales[itemID]++             // 增加物品的销售数量。
	re.lastUpdate[itemID] = time.Now() // 更新物品的最后活动时间。
}

// UserBasedCF (User-Based Collaborative Filtering) 基于用户的协同过滤推荐。
// 找出与目标用户兴趣相似的其他用户，然后推荐这些相似用户喜欢但目标用户未接触过的物品。
// userID: 目标用户ID。
// topN: 返回的推荐物品数量。
func (re *RecommendationEngine) UserBasedCF(userID uint64, topN int) []uint64 {
	re.mu.RLock()         // 加读锁。
	defer re.mu.RUnlock() // 确保函数退出时解锁。

	// 找出与目标用户相似的用户。
	similarities := make(map[uint64]float64) // UserID -> SimilarityScore。
	for otherUserID := range re.userItemMatrix {
		if otherUserID != userID { // 排除自己。
			sim := re.cosineSimilarity(userID, otherUserID) // 计算余弦相似度。
			if sim > 0 {
				similarities[otherUserID] = sim
			}
		}
	}

	// 基于相似用户的评分，计算物品的预测评分。
	predictions := make(map[uint64]float64) // ItemID -> PredictedRating。
	userRatings := re.userItemMatrix[userID]

	for otherUserID, similarity := range similarities {
		for itemID, rating := range re.userItemMatrix[otherUserID] {
			// 跳过目标用户已经评分过的物品。
			if _, exists := userRatings[itemID]; exists {
				continue
			}
			// 累加相似用户对该物品的评分，乘以相似度作为权重。
			predictions[itemID] += similarity * rating
		}
	}

	// 排序预测评分并返回TopN物品。
	return re.topNItems(predictions, topN)
}

// ItemBasedCF (Item-Based Collaborative Filtering) 基于物品的协同过滤推荐。
// 找出与目标用户已经喜欢的物品相似的其他物品，然后推荐这些相似物品。
// userID: 目标用户ID。
// topN: 返回的推荐物品数量。
func (re *RecommendationEngine) ItemBasedCF(userID uint64, topN int) []uint64 {
	re.mu.RLock()
	defer re.mu.RUnlock()

	userRatings := re.userItemMatrix[userID]
	if len(userRatings) == 0 {
		return nil // 如果用户没有评分过任何物品，则无法进行基于物品的推荐。
	}

	// 计算物品相似度并预测评分。
	predictions := make(map[uint64]float64) // ItemID -> PredictedRating。

	for ratedItemID := range userRatings { // 遍历用户已评分的物品。
		for candidateItemID := range re.itemUserMatrix { // 遍历所有物品作为候选推荐。
			// 跳过用户已经评分过的物品。
			if _, exists := userRatings[candidateItemID]; exists {
				continue
			}

			// 计算已评分物品与候选物品的相似度。
			similarity := re.itemSimilarity(ratedItemID, candidateItemID)
			if similarity > 0 {
				// 预测评分 = 相似度 * 用户对已评分物品的评分。
				predictions[candidateItemID] += similarity * userRatings[ratedItemID]
			}
		}
	}

	return re.topNItems(predictions, topN)
}

// HotItems 热门商品推荐。
// 基于物品的浏览量和销量计算热度，并加入时间衰减因子，推荐当前最热门的物品。
// topN: 返回的推荐物品数量。
// decayHours: 时间衰减的半衰期（以小时计），表示热度衰减的速度。
func (re *RecommendationEngine) HotItems(topN int, decayHours float64) []uint64 {
	re.mu.RLock()
	defer re.mu.RUnlock()

	now := time.Now()
	scores := make(map[uint64]float64) // ItemID -> HotScore。

	for itemID := range re.itemUserMatrix { // 遍历所有物品。
		// 计算物品的基础热度分数。
		viewScore := float64(re.itemViews[itemID])
		saleScore := float64(re.itemSales[itemID]) * 5 // 销量通常比浏览量权重更高。

		baseScore := viewScore + saleScore

		// 应用时间衰减。
		if lastUpdate, exists := re.lastUpdate[itemID]; exists {
			hoursPassed := now.Sub(lastUpdate).Hours()         // 计算自上次更新以来的小时数。
			decayFactor := math.Exp(-hoursPassed / decayHours) // 指数衰减。
			scores[itemID] = baseScore * decayFactor
		} else {
			scores[itemID] = baseScore // 如果没有更新时间，则不进行衰减。
		}
	}

	return re.topNItems(scores, topN)
}

// PersonalizedHot 个性化热门推荐。
// 结合物品的热度（浏览量、销量、时间衰减）和用户的偏好（通过物品相似度计算），
// 为用户推荐个性化的热门物品。
// userID: 目标用户ID。
// topN: 返回的推荐物品数量。
func (re *RecommendationEngine) PersonalizedHot(userID uint64, topN int) []uint64 {
	re.mu.RLock()
	defer re.mu.RUnlock()

	userRatings := re.userItemMatrix[userID]
	// 如果用户没有评分过任何物品，则退化为普通热门推荐（此处简化为返回nil，实际应调用HotItems）。
	if len(userRatings) == 0 {
		return nil
	}

	scores := make(map[uint64]float64) // ItemID -> CombinedScore。
	now := time.Now()

	for itemID := range re.itemUserMatrix { // 遍历所有物品。
		// 跳过用户已经评分过的物品。
		if _, exists := userRatings[itemID]; exists {
			continue
		}

		// 计算物品的热度分数。
		viewScore := float64(re.itemViews[itemID])
		saleScore := float64(re.itemSales[itemID]) * 5
		hotScore := viewScore + saleScore

		// 应用时间衰减。
		decayFactor := 1.0
		if lastUpdate, exists := re.lastUpdate[itemID]; exists {
			hoursPassed := now.Sub(lastUpdate).Hours()
			decayFactor = math.Exp(-hoursPassed / 24.0) // 假设半衰期为24小时。
		}

		// 计算用户对该物品的偏好分数。
		// 通过计算该物品与用户已评分物品的相似度来估计。
		preferenceScore := 0.0
		count := 0
		for ratedItemID := range userRatings {
			sim := re.itemSimilarity(ratedItemID, itemID)
			if sim > 0 {
				preferenceScore += sim
				count++
			}
		}
		if count > 0 {
			preferenceScore /= float64(count) // 取平均相似度。
		}

		// 综合评分：结合热度、个人偏好和时间衰减。
		// 权重示例：热度50%，个人偏好30%，时间衰减20%。
		scores[itemID] = 0.5*hotScore + 0.3*preferenceScore*100 + 0.2*decayFactor*100
	}

	return re.topNItems(scores, topN)
}

// SimilarItems 相似商品推荐。
// 找出与给定物品最相似的其他物品。
// itemID: 目标物品ID。
// topN: 返回的推荐物品数量。
func (re *RecommendationEngine) SimilarItems(itemID uint64, topN int) []uint64 {
	re.mu.RLock()
	defer re.mu.RUnlock()

	similarities := make(map[uint64]float64) // ItemID -> SimilarityScore。

	for candidateItemID := range re.itemUserMatrix { // 遍历所有物品作为候选相似物品。
		if candidateItemID == itemID {
			continue // 排除自己。
		}

		sim := re.itemSimilarity(itemID, candidateItemID) // 计算物品相似度。
		if sim > 0 {
			similarities[candidateItemID] = sim
		}
	}

	return re.topNItems(similarities, topN)
}

// AlsoBought (又称Item-to-Item Collaborative Filtering) 推荐。
// 找出与给定物品经常一起被购买的其他物品。
// itemID: 目标物品ID。
// topN: 返回的推荐物品数量。
func (re *RecommendationEngine) AlsoBought(itemID uint64, topN int) []uint64 {
	re.mu.RLock()
	defer re.mu.RUnlock()

	// 找出所有购买了目标物品的用户。
	users := re.itemUserMatrix[itemID]
	if len(users) == 0 {
		return nil
	}

	// 统计这些用户还购买了哪些其他物品。
	itemCounts := make(map[uint64]int) // ItemID -> Co-occurrenceCount。
	for userID := range users {
		for otherItemID := range re.userItemMatrix[userID] {
			if otherItemID != itemID { // 排除目标物品本身。
				itemCounts[otherItemID]++
			}
		}
	}

	// 转换为float64分数进行排序。
	scores := make(map[uint64]float64)
	for itemID, count := range itemCounts {
		scores[itemID] = float64(count)
	}

	return re.topNItems(scores, topN)
}

// cosineSimilarity 计算两个用户之间的余弦相似度。
// 相似度基于用户对物品的评分向量。
// user1, user2: 两个用户的ID。
func (re *RecommendationEngine) cosineSimilarity(user1, user2 uint64) float64 {
	ratings1 := re.userItemMatrix[user1] // 用户1的评分向量。
	ratings2 := re.userItemMatrix[user2] // 用户2的评分向量。

	if len(ratings1) == 0 || len(ratings2) == 0 {
		return 0 // 任何一个用户没有评分，则相似度为0。
	}

	var dotProduct, norm1, norm2 float64

	// 计算点积和范数。
	for itemID, rating1 := range ratings1 {
		if rating2, exists := ratings2[itemID]; exists { // 仅考虑共同评分的物品。
			dotProduct += rating1 * rating2
		}
		norm1 += rating1 * rating1
	}

	for _, rating2 := range ratings2 { // 考虑到user2可能评分了user1未评分的物品。
		norm2 += rating2 * rating2
	}

	if norm1 == 0 || norm2 == 0 {
		return 0 // 避免除以零。
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2)) // 余弦相似度公式。
}

// itemSimilarity 计算两个物品之间的余弦相似度。
// 相似度基于用户对这两个物品的评分向量。
// item1, item2: 两个物品的ID。
func (re *RecommendationEngine) itemSimilarity(item1, item2 uint64) float64 {
	users1 := re.itemUserMatrix[item1] // 物品1的用户评分向量。
	users2 := re.itemUserMatrix[item2] // 物品2的用户评分向量。

	if len(users1) == 0 || len(users2) == 0 {
		return 0 // 任何一个物品没有用户评分，则相似度为0。
	}

	var dotProduct, norm1, norm2 float64

	// 计算点积和范数。
	for userID, rating1 := range users1 {
		if rating2, exists := users2[userID]; exists { // 仅考虑共同评分的用户。
			dotProduct += rating1 * rating2
		}
		norm1 += rating1 * rating1
	}

	for _, rating2 := range users2 { // 考虑到user2可能评分了user1未评分的物品。
		norm2 += rating2 * rating2
	}

	if norm1 == 0 || norm2 == 0 {
		return 0 // 避免除以零。
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2)) // 余弦相似度公式。
}

// topNItems 是一个辅助函数，用于从一个物品得分map中，选出得分最高的TopN物品ID。
func (re *RecommendationEngine) topNItems(scores map[uint64]float64, topN int) []uint64 {
	type itemScore struct {
		itemID uint64
		score  float64
	}

	items := make([]itemScore, 0, len(scores))
	for itemID, score := range scores {
		items = append(items, itemScore{itemID, score})
	}

	// 按照得分降序排序。
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	result := make([]uint64, 0, topN)
	for i := 0; i < len(items) && i < topN; i++ {
		result = append(result, items[i].itemID)
	}

	return result
}

// UpdateItemScore 更新单个物品的综合得分。
// 这个函数通常在物品数据发生变化时被调用。
// itemID: 要更新的物品ID。
func (re *RecommendationEngine) UpdateItemScore(itemID uint64) {
	re.mu.Lock()
	defer re.mu.Unlock()

	viewScore := float64(re.itemViews[itemID])
	saleScore := float64(re.itemSales[itemID]) * 5 // 销量权重更高。

	// 计算时间衰减因子。
	decayFactor := 1.0
	if lastUpdate, exists := re.lastUpdate[itemID]; exists {
		hoursPassed := time.Since(lastUpdate).Hours()
		decayFactor = math.Exp(-hoursPassed / 24.0) // 假设半衰期为24小时。
	}

	re.itemScores[itemID] = (viewScore + saleScore) * decayFactor // 更新物品综合得分。
}

// BatchUpdateScores 批量更新所有物品的综合得分。
// 这个函数通常由后台定时任务调用，以定期更新所有物品的热度得分。
func (re *RecommendationEngine) BatchUpdateScores() {
	re.mu.Lock()
	defer re.mu.Unlock()

	for itemID := range re.itemUserMatrix { // 遍历所有物品。
		viewScore := float64(re.itemViews[itemID])
		saleScore := float64(re.itemSales[itemID]) * 5

		decayFactor := 1.0
		if lastUpdate, exists := re.lastUpdate[itemID]; exists {
			hoursPassed := time.Since(lastUpdate).Hours()
			decayFactor = math.Exp(-hoursPassed / 24.0) // 假设半衰期为24小时。
		}

		re.itemScores[itemID] = (viewScore + saleScore) * decayFactor
	}
}
