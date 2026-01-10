// Package algorithm 提供了高性能算法实现.
package algorithm

import (
	"errors"
	"math"
)

var (
	// ErrDimMismatchBounds 维度不匹配.
	ErrDimMismatchBounds = errors.New("constraints and bounds dimensions mismatch")
	// ErrUnboundedProblem 问题无界.
	ErrUnboundedProblem = errors.New("unbounded problem")
)

const lpEpsilon = 1e-9

// LinearProgramming 结构体实现了标准的单纯形法 (Simplex Method).
type LinearProgramming struct {
	tableau          [][]float64
	constraintsCount int
	variablesCount   int
}

// NewLinearProgramming 初始化一个单纯形求解器.
func NewLinearProgramming(objective []float64, constraints [][]float64, bounds []float64) (*LinearProgramming, error) {
	numConstraints := len(constraints)
	numVariables := len(objective)

	if numConstraints != len(bounds) {
		return nil, ErrDimMismatchBounds
	}

	tableau := make([][]float64, numConstraints+1)
	for i := range tableau {
		tableau[i] = make([]float64, numVariables+numConstraints+1)
	}

	for i := range numConstraints {
		for j := range numVariables {
			tableau[i][j] = constraints[i][j]
		}

		tableau[i][numVariables+i] = 1.0
		tableau[i][numVariables+numConstraints] = bounds[i]
	}

	for j := range numVariables {
		tableau[numConstraints][j] = -objective[j]
	}

	return &LinearProgramming{
		constraintsCount: numConstraints,
		variablesCount:   numVariables,
		tableau:          tableau,
	}, nil
}

// Solve 执行单纯形迭代求解.
func (lp *LinearProgramming) Solve() (solution []float64, value float64, err error) {
	for {
		pivotCol := lp.findPivotColumn()
		if pivotCol == -1 {
			break
		}

		pivotRow := lp.findPivotRow(pivotCol)
		if pivotRow == -1 {
			return nil, 0, ErrUnboundedProblem
		}

		lp.pivot(pivotRow, pivotCol)
	}

	resSol := lp.extractSolution()
	resVal := lp.tableau[lp.constraintsCount][lp.variablesCount+lp.constraintsCount]

	return resSol, resVal, nil
}

func (lp *LinearProgramming) findPivotColumn() int {
	pivotCol := -1
	minVal := -lpEpsilon

	targetRowIdx := lp.constraintsCount
	limit := lp.variablesCount + lp.constraintsCount

	for j := range limit {
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

	for i := range lp.constraintsCount {
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

	for j := range limit + 1 {
		lp.tableau[row][j] /= pivotVal
	}

	for i := range lp.constraintsCount + 1 {
		if i != row {
			multiplier := lp.tableau[i][col]
			for j := range limit + 1 {
				lp.tableau[i][j] -= multiplier * lp.tableau[row][j]
			}
		}
	}
}

func (lp *LinearProgramming) extractSolution() []float64 {
	solution := make([]float64, lp.variablesCount)
	constColIdx := lp.variablesCount + lp.constraintsCount

	for j := range lp.variablesCount {
		row := lp.findBasicVariableRow(j)
		if row >= 0 {
			solution[j] = lp.tableau[row][constColIdx]
		}
	}

	return solution
}

func (lp *LinearProgramming) findBasicVariableRow(col int) int {
	targetRow := -1

	for i := range lp.constraintsCount {
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
