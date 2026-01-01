package algorithm

import "math"

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

// SimplexMethod 使用单纯形法求解线性规划问题（标准最大化问题）。
// 标准形式：Max Z = CX, 满足 AX <= B 且 X >= 0。
// 返回决策变量的最优解切片。
func (lp *LinearProgramming) SimplexMethod() []float64 {
	// 1. 构建初始单纯形大表 (Tableau)
	// 行数 = 约束数 + 1 (目标函数行)
	// 列数 = 决策变量数 + 松弛变量数 + 1 (右侧常数项)
	numSlack := lp.numConstraints
	rows := lp.numConstraints + 1
	cols := lp.numVars + numSlack + 1
	tableau := make([][]float64, rows)
	for i := range tableau {
		tableau[i] = make([]float64, cols)
	}

	// 填充约束条件
	for i := 0; i < lp.numConstraints; i++ {
		for j := 0; j < lp.numVars; j++ {
			tableau[i][j] = lp.constraints[i][j]
		}
		// 填充松弛变量 (单位矩阵)
		tableau[i][lp.numVars+i] = 1
		// 填充右侧常数
		tableau[i][cols-1] = lp.bounds[i]
	}

	// 填充目标函数行 (最后一行)
	// 形式为: Z - c1x1 - c2x2 ... = 0
	for j := 0; j < lp.numVars; j++ {
		tableau[rows-1][j] = -lp.objective[j]
	}

	// 2. 迭代寻优
	for {
		// 找到入基变量：寻找最后一行中绝对值最大的负数
		pivotCol := -1
		minVal := 0.0
		for j := 0; j < cols-1; j++ {
			if tableau[rows-1][j] < minVal {
				minVal = tableau[rows-1][j]
				pivotCol = j
			}
		}

		// 如果没有负系数，说明已达到最优解
		if pivotCol == -1 {
			break
		}

		// 找到出基变量：计算最小比率 (Min Ratio Test)
		pivotRow := -1
		minRatio := math.MaxFloat64
		for i := 0; i < lp.numConstraints; i++ {
			if tableau[i][pivotCol] > 0 {
				ratio := tableau[i][cols-1] / tableau[i][pivotCol]
				if ratio < minRatio {
					minRatio = ratio
					pivotRow = i
				}
			}
		}

		// 如果无法找到出基变量，说明问题无界
		if pivotRow == -1 {
			return nil
		}

		// 3. 枢轴转动 (Pivoting)
		pivotVal := tableau[pivotRow][pivotCol]
		// 归一化枢轴行
		for j := 0; j < cols; j++ {
			tableau[pivotRow][j] /= pivotVal
		}

		// 消除其他行
		for i := 0; i < rows; i++ {
			if i != pivotRow {
				factor := tableau[i][pivotCol]
				for j := 0; j < cols; j++ {
					tableau[i][j] -= factor * tableau[pivotRow][j]
				}
			}
		}
	}

	// 4. 提取决策变量的解
	solution := make([]float64, lp.numVars)
	for j := 0; j < lp.numVars; j++ {
		isBasic := false
		rowIndex := -1
		for i := 0; i < lp.numConstraints; i++ {
			if tableau[i][j] == 1 {
				if rowIndex == -1 {
					rowIndex = i
					isBasic = true
				} else {
					isBasic = false // 这一列有多个 1
					break
				}
			} else if tableau[i][j] != 0 {
				isBasic = false // 这一列包含非 0/1 的值
				break
			}
		}
		if isBasic && rowIndex != -1 {
			solution[j] = tableau[rowIndex][cols-1]
		}
	}

	return solution
}
