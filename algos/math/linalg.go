package math

import (
	"math"

	"github.com/wyfcoding/pkg/xerrors"
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
		return nil, xerrors.ErrEmptyData
	}

	cols := len(data[0])
	mat := NewMatrix(rows, cols)

	for i := range rows {
		if len(data[i]) != cols {
			return nil, xerrors.ErrDimMismatch
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
		return nil, xerrors.ErrDimMismatch
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
		return nil, xerrors.ErrDimMismatch
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
		return nil, xerrors.ErrNotSquare
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
					return nil, xerrors.ErrNotPositiveDefinite
				}

				res.Set(i, j, math.Sqrt(val))
			} else {
				res.Set(i, j, (m.Get(i, j)-sum)/res.Get(j, j))
			}
		}
	}

	return res, nil
}

// ForwardSubstitute 解下三角方程组 Ly = b.
func (m *Matrix) ForwardSubstitute(b []float64) ([]float64, error) {
	if m.Rows != m.Cols || len(b) != m.Rows {
		return nil, xerrors.ErrDimMismatch
	}

	res := make([]float64, m.Rows)
	for i := range m.Rows {
		var sum float64
		for j := range i {
			sum += m.Get(i, j) * res[j]
		}
		res[i] = (b[i] - sum) / m.Get(i, i)
	}

	return res, nil
}

// BackwardSubstitute 解上三角方程组 Ux = y (U 是 L^T).
func (m *Matrix) BackwardSubstitute(b []float64) ([]float64, error) {
	if m.Rows != m.Cols || len(b) != m.Rows {
		return nil, xerrors.ErrDimMismatch
	}

	res := make([]float64, m.Rows)
	for i := m.Rows - 1; i >= 0; i-- {
		var sum float64
		for j := i + 1; j < m.Cols; j++ {
			sum += m.Get(j, i) * res[j] // 这里使用 L 的转置，即原本的 L.Get(j, i)
		}
		res[i] = (b[i] - sum) / m.Get(i, i)
	}

	return res, nil
}

// SolveCholesky 使用 Cholesky 分解求解 Mx = b.
func (m *Matrix) SolveCholesky(b []float64) ([]float64, error) {
	L, err := m.Cholesky()
	if err != nil {
		return nil, err
	}

	y, err := L.ForwardSubstitute(b)
	if err != nil {
		return nil, err
	}

	return L.BackwardSubstitute(y)
}
