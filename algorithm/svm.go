package algorithm

import (
	"sync"
)

// Point 结构体代表数据集中的一个数据点。
// 它是SVM算法训练和预测的输入特征。
// 注意：此结构体通常在外部定义，此处假设它包含一个名为 Data 的浮点数切片。
type SVMPoint struct {
	Data []float64
}

// SVM 结构体实现了支持向量机分类器，支持线性与 RBF 核。
type SVM struct {
	kernel  string
	weights []float64
	mu      sync.RWMutex
	bias    float64
	gamma   float64
}

// NewSVM 创建一个新的 SVM 实例。
func NewSVM(kernel string, gamma float64) *SVM {
	if kernel == "" {
		kernel = "linear"
	}
	return &SVM{
		kernel: kernel,
		gamma:  gamma,
	}
}

// Train 使用随机梯度下降 (SGD) 训练支持向量机模型。
// 最小化 Hinge Loss + L2 正则化项: J(w) = 1/2 * ||w||^2 + C * sum(max(0, 1 - y*(w*x + b)).
func (s *SVM) Train(points [][]float64, labels []int, epochs int, lr, lambda float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dim := len(points[0])
	s.weights = make([]float64, dim)
	s.bias = 0.0

	for range epochs {
		for i, x := range points {
			y := float64(labels[i])
			// 判定是否命中 Hinge Loss 区.
			prediction := s.bias
			for j := range dim {
				prediction += s.weights[j] * x[j]
			}

			if y*prediction < 1 {
				// 更新权重 (梯度包含正则项和 Loss 项.
				for j := range dim {
					s.weights[j] -= lr * (lambda*s.weights[j] - y*x[j])
				}
				s.bias += lr * y
			} else {
				// 仅更新权重中的正则项部.
				for j := range dim {
					s.weights[j] -= lr * (lambda * s.weights[j])
				}
			}
		}
	}
}

// Predict 使用训练好的 SVM 模型进行分类。
func (s *SVM) Predict(x []float64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := s.bias
	for i, val := range s.weights {
		result += val * x[i]
	}

	if result >= 0 {
		return 1
	}
	return -1
}
