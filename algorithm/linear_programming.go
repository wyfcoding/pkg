package algorithm

// LinearProgramming 结构体定义了一个线性规划问题。
// 线性规划是一种在给定一组线性约束条件下，最大化或最小化线性目标函数的数学优化技术。
// 在本实现中，`SimplexMethod` 提供了一个简化的贪心近似求解方法。
type LinearProgramming struct {
	objective      []float64   // 目标函数系数：例如，max (c1*x1 + c2*x2 + ...)。
	constraints    [][]float64 // 约束条件矩阵：例如，a11*x1 + a12*x2 <= b1。
	bounds         []float64   // 约束条件的右侧值（b值）。
	numVars        int         // 决策变量的数量。
	numConstraints int         // 约束条件的数量。
}

// NewLinearProgramming 创建并返回一个新的 LinearProgramming 求解器实例。
// numVars: 决策变量的数量。
// numConstraints: 约束条件（不包括非负约束）的数量。
func NewLinearProgramming(numVars, numConstraints int) *LinearProgramming {
	return &LinearProgramming{
		objective:      make([]float64, numVars),
		constraints:    make([][]float64, numConstraints),
		bounds:         make([]float64, numConstraints),
		numVars:        numVars,
		numConstraints: numConstraints,
	}
}

// SetObjective 设置线性规划的目标函数系数。
// coeffs: 与每个决策变量对应的系数。
func (lp *LinearProgramming) SetObjective(coeffs []float64) {
	// 确保传入的系数数量与定义的变量数量一致。
	if len(coeffs) != lp.numVars {
		panic("目标函数系数的数量与变量数量不匹配")
	}
	copy(lp.objective, coeffs)
}

// AddConstraint 在指定索引处添加一个约束条件。
// idx: 约束条件的索引。
// coeffs: 约束条件中每个决策变量的系数。
// bound: 约束条件的右侧值。
func (lp *LinearProgramming) AddConstraint(idx int, coeffs []float64, bound float64) {
	// 确保传入的系数数量与定义的变量数量一致。
	if len(coeffs) != lp.numVars {
		panic("约束系数的数量与变量数量不匹配")
	}
	lp.constraints[idx] = make([]float64, len(coeffs))
	copy(lp.constraints[idx], coeffs)
	lp.bounds[idx] = bound
}

// SimplexMethod 尝试使用单纯形法求解线性规划问题。
// 注意：此实现是一个高度简化的贪心近似，并非完整的单纯形算法。
// 它仅适用于特定类型的线性规划问题（例如，所有目标函数系数为正数时，尝试最大化目标函数），
// 并且不保证找到最优解。完整的单纯形法实现要复杂得多。
// 应用场景：例如，在库存优化中，分配有限资源以最大化利润或最小化成本。
// 返回一个包含每个决策变量解的切片。
func (lp *LinearProgramming) SimplexMethod() []float64 {
	// 初始化解向量，所有变量的初始解为0。
	solution := make([]float64, lp.numVars)

	// 计算每个变量的“效益/成本比”。
	// 在这个简化的贪心近似中，直接使用目标函数系数作为“效益”。
	ratios := make([]float64, lp.numVars)
	for i := 0; i < lp.numVars; i++ {
		if lp.objective[i] > 0 { // 假设我们总是尝试最大化正效益的变量。
			ratios[i] = lp.objective[i]
		}
		// 如果目标函数系数为负，则表示该变量会降低效益，或者这是一个最小化问题，
		// 这种简化算法可能无法正确处理。
	}

	// 贪心分配策略：
	// 简单地将具有正效益（目标函数系数）的变量设置为其效益值。
	// 这完全忽略了约束条件，因此这只是一个非常粗略的近似。
	for i := 0; i < lp.numVars; i++ {
		if ratios[i] > 0 {
			solution[i] = ratios[i]
		}
	}

	return solution // 返回近似解。
}
