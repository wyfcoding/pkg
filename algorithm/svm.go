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
// 此处使用简化的梯度下降（或感知机算法的变种）进行训练。
// 应用场景：用户分类、异常检测、文本分类等。
// points: 训练数据集的特征点切片。
// labels: 每个数据点对应的真实标签（预期为0或1，此处会转换为-1或1）。
// learningRate: 学习率，控制每次权重和偏置更新的步长。
// iterations: 训练迭代次数（epochs）。
func (svm *SVM) Train(points []*SVMPoint, labels []int, learningRate float64, iterations int) {
	svm.mu.Lock()         // 训练过程需要加写锁。
	defer svm.mu.Unlock() // 确保函数退出时解锁。

	dim := len(points[0].Data) // 输入特征的维度。

	for iter := 0; iter < iterations; iter++ {
		for i, p := range points {
			// 计算当前数据点的预测值。
			// 预测值 = 权重向量与特征向量的点积 + 偏置项。
			pred := svm.bias
			for j := 0; j < dim; j++ {
				pred += svm.weights[j] * p.Data[j]
			}

			// 将标签从 0/1 转换为 -1/1，以适应SVM的损失函数（通常是合页损失）。
			label := float64(labels[i])
			if label == 0 {
				label = -1 // 将标签0转换为-1。
			}

			// 计算误差（这里是预测值与真实标签的差值，通常SVM会使用 (1 - y * pred) 作为误差项）。
			error := label - pred // 这是一个简化的误差计算。

			// 更新权重。
			// 权重更新 = 学习率 * 误差 * 特征值。
			for j := 0; j < dim; j++ {
				svm.weights[j] += learningRate * error * p.Data[j]
			}

			// 更新偏置。
			// 偏置更新 = 学习率 * 误差。
			svm.bias += learningRate * error
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
