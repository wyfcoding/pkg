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

// SVM 结构体实现了简化的支持向量机（Support Vector Machine）分类器。
// SVM 是一种二分类模型，它旨在找到一个超平面，将不同类别的样本尽可能大地分开。
// 此处实现的训练方法是简化的梯度下降，而非经典的SMO（Sequential Minimal Optimization）算法。
type SVM struct {
	weights []float64    // 权重向量，定义了分类超平面的法向量。
	bias    float64      // 偏置项，定义了超平面与原点的距离。
	mu      sync.RWMutex // 读写锁，用于在训练和预测时的并发安全。
}

// NewSVM 创建并返回一个新的 SVM 实例。
// dim: 输入特征的维度。
func NewSVM(dim int) *SVM {
	return &SVM{
		weights: make([]float64, dim), // 初始化权重向量为零。
		bias:    0,                    // 初始化偏置项为零。
	}
}

// Train 使用随机梯度下降 (SGD) 训练支持向量机模型。
// 最小化 Hinge Loss + L2 正则化项: J(w) = 1/2 * ||w||^2 + C * sum(max(0, 1 - y*(w*x + b)))
func (s *SVM) Train(points [][]float64, labels []int, epochs int, lr float64, lambda float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dim := len(points[0])
	s.weights = make([]float64, dim)
	s.bias = 0.0

	for epoch := 0; epoch < epochs; epoch++ {
		for i, x := range points {
			y := float64(labels[i])
			// 判定是否命中 Hinge Loss 区域
			prediction := s.bias
			for j := 0; j < dim; j++ {
				prediction += s.weights[j] * x[j]
			}

			if y*prediction < 1 {
				// 更新权重 (梯度包含正则项和 Loss 项)
				for j := 0; j < dim; j++ {
					s.weights[j] = s.weights[j] - lr*(lambda*s.weights[j]-y*x[j])
				}
				s.bias += lr * y
			} else {
				// 仅更新权重中的正则项部分
				for j := 0; j < dim; j++ {
					s.weights[j] = s.weights[j] - lr*(lambda*s.weights[j])
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
