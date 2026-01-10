// Package algorithm 提供了高性能算法实现。
package algorithm

import (
	"errors"
	"math"
)

// LinearProgramming 结构体实现了标准的单纯形法 (Simplex Method)。
type LinearProgramming struct {
	constraintsCount int         // 约束个数。
	variablesCount   int         // 变量个数。
	tableau          [][]float64 // 单纯形表。
}

const lpEpsilon = 1e-9

// NewLinearProgramming 初始化一个单纯形求解器。
func NewLinearProgramming(objective []float64, constraints [][]float64, bounds []float64) (*LinearProgramming, error) {
	m := len(constraints)
	n := len(objective)

	if m != len(bounds) {
		return nil, errors.New("constraints and bounds dimensions mismatch")
	}

	tableau := make([][]float64, m+1)
	for i := range tableau {
		tableau[i] = make([]float64, n+m+1)
	}

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			tableau[i][j] = constraints[i][j]
		}

		tableau[i][n+i] = 1.0
		tableau[i][n+m] = bounds[i]
	}

	for j := 0; j < n; j++ {
		tableau[m][j] = -objective[j]
	}

	return &LinearProgramming{
		constraintsCount: m,
		variablesCount:   n,
		tableau:          tableau,
	}, nil
}

// Solve 执行单纯形迭代求解。
func (lp *LinearProgramming) Solve() ([]float64, float64, error) {
	for {
		pivotCol := lp.findPivotColumn()
		if pivotCol == -1 {
			break
		}

		pivotRow := lp.findPivotRow(pivotCol)
		if pivotRow == -1 {
			return nil, 0, errors.New("unbounded problem")
		}

		lp.pivot(pivotRow, pivotCol)
	}

	return lp.extractSolution(), lp.tableau[lp.constraintsCount][lp.variablesCount+lp.constraintsCount], nil
}

func (lp *LinearProgramming) findPivotColumn() int {
	pivotCol := -1
	minVal := -lpEpsilon

	targetRowIdx := lp.constraintsCount
	limit := lp.variablesCount + lp.constraintsCount

	for j := 0; j < limit; j++ {
		if lp.tableau[targetRowIdx][j] < minVal {
			minVal = lp.tableau[targetRowIdx][j]
			pivotCol = j
		}
	}

	return pivotCol
}

func (lp *LinearProgramming) findPivotRow(pivotCol int) int {
	pivotRow := -1
	minRatio := math.MaxFloat64

	constColIdx := lp.variablesCount + lp.constraintsCount

	for i := 0; i < lp.constraintsCount; i++ {
		if lp.tableau[i][pivotCol] > lpEpsilon {
			ratio := lp.tableau[i][constColIdx] / lp.tableau[i][pivotCol]
			if ratio < minRatio {
				minRatio = ratio
				pivotRow = i
			}
		}
	}

	return pivotRow
}

func (lp *LinearProgramming) pivot(row, col int) {
	pivotVal := lp.tableau[row][col]
	limit := lp.variablesCount + lp.constraintsCount

	for j := 0; j <= limit; j++ {
		lp.tableau[row][j] /= pivotVal
	}

	for i := 0; i <= lp.constraintsCount; i++ {
		if i != row {
			multiplier := lp.tableau[i][col]
			for j := 0; j <= limit; j++ {
				lp.tableau[i][j] -= multiplier * lp.tableau[row][j]
			}
		}
	}
}

func (lp *LinearProgramming) extractSolution() []float64 {
	solution := make([]float64, lp.variablesCount)
	constColIdx := lp.variablesCount + lp.constraintsCount

	for j := 0; j < lp.variablesCount; j++ {
		row := lp.findBasicVariableRow(j)
		if row >= 0 {
			solution[j] = lp.tableau[row][constColIdx]
		}
	}

	return solution
}

func (lp *LinearProgramming) findBasicVariableRow(col int) int {
	targetRow := -1

	for i := 0; i < lp.constraintsCount; i++ {
		val := lp.tableau[i][col]
		if math.Abs(val-1.0) < lpEpsilon {
			if targetRow != -1 {
				return -1
			}

			targetRow = i
		} else if math.Abs(val) > lpEpsilon {
			return -1
		}
	}

	return targetRow
}