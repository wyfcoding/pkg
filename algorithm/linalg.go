package algorithm

import (
	"errors"
	"math"
)

// Matrix 定义基础矩阵结构
type Matrix struct {
	Rows int
	Cols int
	Data []float64 // 行优先存储
}

// NewMatrix 创建一个 r x c 的零矩阵
func NewMatrix(r, c int) *Matrix {
	return &Matrix{
		Rows: r,
		Cols: c,
		Data: make([]float64, r*c),
	}
}

// NewMatrixFromData 从二维切片创建矩阵
func NewMatrixFromData(data [][]float64) (*Matrix, error) {
	r := len(data)
	if r == 0 {
		return nil, errors.New("empty data")
	}
	c := len(data[0])
	m := NewMatrix(r, c)
	for i := range r {
		if len(data[i]) != c {
			return nil, errors.New("columns dimension mismatch")
		}
		for j := range c {
			m.Set(i, j, data[i][j])
		}
	}
	return m, nil
}

// Get 获取元素 (i, j)
func (m *Matrix) Get(i, j int) float64 {
	return m.Data[i*m.Cols+j]
}

// Set 设置元素 (i, j)
func (m *Matrix) Set(i, j int, v float64) {
	m.Data[i*m.Cols+j] = v
}

// Transpose 矩阵转置
func (m *Matrix) Transpose() *Matrix {
	res := NewMatrix(m.Cols, m.Rows)
	for i := 0; i < m.Rows; i++ {
		for j := 0; j < m.Cols; j++ {
			res.Set(j, i, m.Get(i, j))
		}
	}
	return res
}

// MultiplyVector 矩阵向量乘法: y = A * x
func (m *Matrix) MultiplyVector(x []float64) ([]float64, error) {
	if len(x) != m.Cols {
		return nil, errors.New("dimension mismatch")
	}
	y := make([]float64, m.Rows)
	for i := 0; i < m.Rows; i++ {
		sum := 0.0
		for j := 0; j < m.Cols; j++ {
			sum += m.Get(i, j) * x[j]
		}
		y[i] = sum
	}
	return y, nil
}

// Cholesky 分解: A = L * L^T
// 返回下三角矩阵 L。A 必须是对称正定矩阵。
// 使用 Cholesky–Banachiewicz 算法。
func (m *Matrix) Cholesky() (*Matrix, error) {
	if m.Rows != m.Cols {
		return nil, errors.New("matrix must be square")
	}
	n := m.Rows
	l := NewMatrix(n, n)

	for i := range n {
		for j := 0; j <= i; j++ {
			sum := 0.0
			for k := 0; k < j; k++ {
				sum += l.Get(i, k) * l.Get(j, k)
			}

			if i == j {
				// 对角线元素
				val := m.Get(i, i) - sum
				if val <= 0 {
					return nil, errors.New("matrix is not positive definite")
				}
				l.Set(i, j, math.Sqrt(val))
			} else {
				// 非对角线元素
				l.Set(i, j, (m.Get(i, j)-sum)/l.Get(j, j))
			}
		}
	}
	return l, nil
}
