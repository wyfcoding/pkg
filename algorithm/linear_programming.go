package algorithm

import (
	"errors"
	"math"
)

// LinearProgramming 结构体实现了标准的单纯形法 (Simplex Method)。
// 用于解决形如 Max: c*x subject to: Ax <= b, x >= 0 的线性规划问题。
type LinearProgramming struct {
	m, n    int         // m 是约束个数，n 是变量个数
	tableau [][]float64 // 单纯形表
}

// NewLinearProgramming 初始化一个单纯形求解器。
// objective: 目标函数系数 (c)
// constraints: 约束矩阵 (A)
// bounds: 约束上限 (b)
func NewLinearProgramming(objective []float64, constraints [][]float64, bounds []float64) (*LinearProgramming, error) {
	m := len(constraints)
	n := len(objective)
	if m != len(bounds) {
		return nil, errors.New("constraints and bounds dimensions mismatch")
	}

	// 单纯形表维度: (m+1) x (n+m+1)
	// 包含目标函数行、约束行、松弛变量和常数项
	t := make([][]float64, m+1)
	for i := range t {
		t[i] = make([]float64, n+m+1)
	}

	// 填充约束部分
	for i := range m {
		for j := range n {
			t[i][j] = constraints[i][j]
		}
		t[i][n+i] = 1.0 // 松弛变量
		t[i][n+m] = bounds[i]
	}

	// 填充目标函数部分 (Max Z, 转化为 Z - cx = 0)
	for j := range n {
		t[m][j] = -objective[j]
	}

	return &LinearProgramming{m: m, n: n, tableau: t}, nil
}

// Solve 执行单纯形迭代求解。
// 优化：引入 Epsilon 防止浮点数误差，并隐式使用 Bland 规则（通过按顺序遍历并保持 minRatio <）防止循环。
func (lp *LinearProgramming) Solve() ([]float64, float64, error) {
	const epsilon = 1e-9

	for {
		// 1. 寻找入基变量 (目标行中系数最小的负数列 - Steepest Edge)
		pivotCol := -1
		minVal := -epsilon // 只考虑显著小于 0 的系数
		for j := 0; j < lp.n+lp.m; j++ {
			if lp.tableau[lp.m][j] < minVal {
				minVal = lp.tableau[lp.m][j]
				pivotCol = j
			}
		}

		if pivotCol == -1 {
			break // 所有目标系数非负，已找到最优解
		}

		// 2. 寻找出基变量 (最小比率原则: b_i / a_ij)
		pivotRow := -1
		minRatio := math.MaxFloat64
		for i := 0; i < lp.m; i++ {
			// 只考虑正系数行，防止除以零或负数（维持可行性）
			if lp.tableau[i][pivotCol] > epsilon {
				ratio := lp.tableau[i][lp.n+lp.m] / lp.tableau[i][pivotCol]
				// 使用 < 保证在比率相同时选择索引最小的行 (Bland's Rule Part 2)
				if ratio < minRatio {
					minRatio = ratio
					pivotRow = i
				}
			}
		}

		if pivotRow == -1 {
			return nil, 0, errors.New("unbounded problem")
		}

		// 3. 执行枢轴旋转 (Pivoting)
		lp.pivot(pivotRow, pivotCol)
	}

	// 4. 提取结果
	solution := make([]float64, lp.n)
	for j := 0; j < lp.n; j++ {
		row := -1
		for i := 0; i < lp.m; i++ {
			if math.Abs(lp.tableau[i][j]-1.0) < epsilon {
				if row != -1 {
					row = -2 // 非基变量 (列中有多个 1)
					break
				}
				row = i
			} else if math.Abs(lp.tableau[i][j]) > epsilon {
				row = -2 // 非基变量 (列中有非 0 非 1 值)
				break
			}
		}
		if row >= 0 {
			solution[j] = lp.tableau[row][lp.n+lp.m]
		}
	}

	return solution, lp.tableau[lp.m][lp.n+lp.m], nil
}

func (lp *LinearProgramming) pivot(row, col int) {
	pivotVal := lp.tableau[row][col]
	// 归一化枢轴行
	for j := 0; j <= lp.n+lp.m; j++ {
		lp.tableau[row][j] /= pivotVal
	}

	// 消去其他行中的该列系数
	for i := 0; i <= lp.m; i++ {
		if i != row {
			multiplier := lp.tableau[i][col]
			for j := 0; j <= lp.n+lp.m; j++ {
				lp.tableau[i][j] -= multiplier * lp.tableau[row][j]
			}
		}
	}
}
