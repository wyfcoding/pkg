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

// Train 训练SVM模型。
// 升级实现：使用带 L2 正则化的合页损失 (Hinge Loss) 随机梯度下降 (SGD)。
func (svm *SVM) Train(points []*SVMPoint, labels []int, learningRate float64, iterations int) {
	svm.mu.Lock()
	defer svm.mu.Unlock()

	dim := len(svm.weights)
	lambda := 0.01 // 正则化参数

	for range iterations {
		for i, p := range points {
			// 将标签从 0/1 转换为 -1/1
			label := float64(labels[i])
			if label == 0 {
				label = -1
			}

			// 计算 y * (w * x + b)
			dotProduct := svm.bias
			for j := range dim {
				dotProduct += svm.weights[j] * p.Data[j]
			}

			// 合页损失梯度下降更新规则
			// 如果 label * pred < 1, 说明该点在间隔内或分类错误
			if label*dotProduct < 1 {
				// 更新权重: w = w + lr * (label * x - 2 * lambda * w)
				for j := range dim {
					svm.weights[j] += learningRate * (label*p.Data[j] - 2*lambda*svm.weights[j])
				}
				// 更新偏置
				svm.bias += learningRate * label
			} else {
				// 如果分类正确且在间隔外，仅应用正则化项（权重衰减）
				for j := range dim {
					svm.weights[j] += learningRate * (-2 * lambda * svm.weights[j])
				}
			}
		}
	}
}

// Predict 使用训练好的SVM模型对新的数据点进行预测。
// data: 待预测数据点的特征向量。
// 返回预测的类别标签（0或1）。
func (svm *SVM) Predict(data []float64) int {
	svm.mu.RLock()         // 预测过程只需要读锁。
	defer svm.mu.RUnlock() // 确保函数退出时解锁。

	// 计算预测值。
	pred := svm.bias
	for i := range data {
		pred += svm.weights[i] * data[i]
	}

	// 根据预测值的正负判断类别。
	// 如果预测值 >= 0，则预测为类别1。
	// 如果预测值 < 0，则预测为类别0。
	if pred >= 0 {
		return 1
	}
	return 0
}
