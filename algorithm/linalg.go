package algorithm

import (
	"errors"
	"math"
)

var (
	// ErrEmptyData 矩阵数据不能为空.
	ErrEmptyData = errors.New("empty data")
	// ErrDimMismatch 维度不匹配.
	ErrDimMismatch = errors.New("dimension mismatch")
	// ErrNotSquare 不是方阵.
	ErrNotSquare = errors.New("matrix must be square")
	// ErrNotPositiveDefinite 不是正定矩阵.
	ErrNotPositiveDefinite = errors.New("matrix is not positive definite")
)

// Matrix 定义基础矩阵结构.
type Matrix struct {
	Data []float64
	Rows int
	Cols int
}

// NewMatrix 创建一个 r x c 的零矩阵.
func NewMatrix(rows, cols int) *Matrix {
	return &Matrix{
		Rows: rows,
		Cols: cols,
		Data: make([]float64, rows*cols),
	}
}

// NewMatrixFromData 从二维切片创建矩阵.
func NewMatrixFromData(data [][]float64) (*Matrix, error) {
	rows := len(data)
	if rows == 0 {
		return nil, ErrEmptyData
	}

	cols := len(data[0])
	mat := NewMatrix(rows, cols)

	for i := range rows {
		if len(data[i]) != cols {
			return nil, ErrDimMismatch
		}

		for j := range cols {
			mat.Set(i, j, data[i][j])
		}
	}

	return mat, nil
}

// Get 获取元素 (i, j).
func (m *Matrix) Get(row, col int) float64 {
	return m.Data[row*m.Cols+col]
}

// Set 设置元素 (i, j).
func (m *Matrix) Set(row, col int, val float64) {
	m.Data[row*m.Cols+col] = val
}

// Transpose 矩阵转置.
func (m *Matrix) Transpose() *Matrix {
	res := NewMatrix(m.Cols, m.Rows)
	for i := range m.Rows {
		for j := range m.Cols {
			res.Set(j, i, m.Get(i, j))
		}
	}

	return res
}

// MultiplyVector 矩阵向量乘法: y = A * x.
func (m *Matrix) MultiplyVector(vec []float64) ([]float64, error) {
	if len(vec) != m.Cols {
		return nil, ErrDimMismatch
	}

	res := make([]float64, m.Rows)
	for i := range m.Rows {
		var sum float64
		rowOffset := i * m.Cols
		for j := range m.Cols {
			sum += m.Data[rowOffset+j] * vec[j]
		}

		res[i] = sum
	}

	return res, nil
}

// Multiply 矩阵乘法: C = A * B.
func (m *Matrix) Multiply(other *Matrix) (*Matrix, error) {
	if m.Cols != other.Rows {
		return nil, ErrDimMismatch
	}

	res := NewMatrix(m.Rows, other.Cols)

	for i := range m.Rows {
		rowOffsetA := i * m.Cols
		rowOffsetC := i * res.Cols

		for k := range m.Cols {
			valA := m.Data[rowOffsetA+k]
			rowOffsetB := k * other.Cols

			for j := range other.Cols {
				res.Data[rowOffsetC+j] += valA * other.Data[rowOffsetB+j]
			}
		}
	}

	return res, nil
}

// Cholesky 分解: A = L * L^T.
func (m *Matrix) Cholesky() (*Matrix, error) {
	if m.Rows != m.Cols {
		return nil, ErrNotSquare
	}

	n := m.Rows
	res := NewMatrix(n, n)

	for i := range n {
		for j := range i + 1 {
			var sum float64
			for k := range j {
				sum += res.Get(i, k) * res.Get(j, k)
			}

			if i == j {
				val := m.Get(i, i) - sum
				if val <= 0 {
					return nil, ErrNotPositiveDefinite
				}

				res.Set(i, j, math.Sqrt(val))
			} else {
				res.Set(i, j, (m.Get(i, j)-sum)/res.Get(j, j))
			}
		}
	}

	return res, nil
}
