package algorithm

// Participant 代表参与匹配的一方（如团员或团长）。
type Participant struct {
	ID          int
	Preferences []int // 对另一方成员的偏好排名（由高到低）。
}

// StableMarriage 实现了 Gale-Shapley 算法。
// 用于寻找两组实体之间的稳定匹配。
type StableMarriage struct {
	groupA []Participant // 提案方（如团员）。
	groupB []Participant // 接收方（如团长）。
	size   int
}

func NewStableMarriage(a, b []Participant) *StableMarriage {
	return &StableMarriage{
		groupA: a,
		groupB: b,
		size:   len(a),
	}
}

// Match 运行算法，返回 groupA[i] 匹配到的 groupB 成员 ID。
func (sm *StableMarriage) Match() map[int]int {
	// 1. 预处理 groupB 的偏好，以便 O(1) 查找。
	// bPrefRank[bID][aID] = rank (越小越优先)。
	bPrefRank := make(map[int]map[int]int)
	for _, p := range sm.groupB {
		bPrefRank[p.ID] = make(map[int]int)
		for rank, aID := range p.Preferences {
			bPrefRank[p.ID][aID] = rank
		}
	}

	// 2. 初始化匹配状态。
	aMatch := make(map[int]int) // aID -> bID。
	bMatch := make(map[int]int) // bID -> aID。
	for i := range sm.groupA {
		aMatch[sm.groupA[i].ID] = -1
	}
	for i := range sm.groupB {
		bMatch[sm.groupB[i].ID] = -1
	}

	// 3. 记录 groupA 每个成员下一个要尝试提议的对象索引。
	aNextProposalIdx := make(map[int]int)

	// 4. 寻找单身的 a 进行提议。
	for {
		var freeA *Participant
		for i := range sm.groupA {
			if aMatch[sm.groupA[i].ID] == -1 && aNextProposalIdx[sm.groupA[i].ID] < len(sm.groupA[i].Preferences) {
				freeA = &sm.groupA[i]
				break
			}
		}

		if freeA == nil {
			break
		}

		// a 向下一个最喜欢的 b 提议。
		bID := freeA.Preferences[aNextProposalIdx[freeA.ID]]
		aNextProposalIdx[freeA.ID]++

		currentOwnerOfB := bMatch[bID]
		if currentOwnerOfB == -1 {
			// b 是单身，暂时在一起。
			aMatch[freeA.ID] = bID
			bMatch[bID] = freeA.ID
		} else if bPrefRank[bID][freeA.ID] < bPrefRank[bID][currentOwnerOfB] {
			// b 已有匹配，看 b 更喜欢谁。
			// b 更喜欢 freeA，抛弃旧爱。
			aMatch[currentOwnerOfB] = -1
			aMatch[freeA.ID] = bID
			bMatch[bID] = freeA.ID
		}
		// 否则 freeA 继续寻找。
	}

	return aMatch
}
