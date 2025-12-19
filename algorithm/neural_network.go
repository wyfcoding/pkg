package algorithm

import (
	"math"
	"math/rand/v2" // 使用 Go 1.22+ 提供的新的 rand/v2 包。
	"sync"
)

// Point 结构体代表数据集中的一个数据点。
// 它是神经网络训练和预测的输入特征。
// 注意：此结构体通常在外部定义，此处假设它包含一个名为 Data 的浮点数切片。
type NNPoint struct {
	Data []float64
}

// NeuralNetwork 结构体实现了简单的全连接前馈神经网络。
// 它由多个层组成，每层包含权重和偏置。
type NeuralNetwork struct {
	layers []*Layer     // 神经网络的层列表。
	mu     sync.RWMutex // 读写锁，用于在训练和预测时的并发安全。
}

// Layer 结构体代表神经网络中的一个层。
type Layer struct {
	weights [][]float64 // 权重矩阵，连接当前层神经元和下一层神经元。
	bias    []float64   // 偏置向量，为下一层每个神经元增加一个偏移量。
	output  []float64   // 当前层的输出值（在前向传播过程中计算）。
}

// NewNeuralNetwork 创建并返回一个新的 NeuralNetwork 实例。
// layerSizes: 一个整数切片，定义了神经网络中每一层的神经元数量。
// 例如，[2, 3, 1] 表示一个输入层有2个神经元，一个隐藏层有3个神经元，一个输出层有1个神经元。
func NewNeuralNetwork(layerSizes []int) *NeuralNetwork {
	nn := &NeuralNetwork{
		layers: make([]*Layer, len(layerSizes)-1), // 层的数量是 (输入层到输出层) - 1。
	}

	// 遍历并初始化每一层（除了输入层本身）。
	for i := 0; i < len(layerSizes)-1; i++ {
		nn.layers[i] = &Layer{
			weights: make([][]float64, layerSizes[i]), // 权重矩阵的行数等于当前层神经元数量。
			bias:    make([]float64, layerSizes[i+1]), // 偏置向量的长度等于下一层神经元数量。
			output:  make([]float64, layerSizes[i+1]), // 输出向量的长度等于下一层神经元数量。
		}

		// 随机初始化权重。
		// 权重通常初始化为接近0的小随机数，以打破对称性，帮助模型学习。
		for j := 0; j < layerSizes[i]; j++ {
			nn.layers[i].weights[j] = make([]float64, layerSizes[i+1])
			for k := 0; k < layerSizes[i+1]; k++ {
				nn.layers[i].weights[j][k] = (rand.Float64() - 0.5) * 2 // 随机值范围在 [-1, 1]。
			}
		}
	}

	return nn
}

// Forward 执行神经网络的前向传播过程。
// input: 神经网络的输入特征向量。
// 返回神经网络的输出结果。
func (nn *NeuralNetwork) Forward(input []float64) []float64 {
	nn.mu.RLock()         // 前向传播只需要读锁。
	defer nn.mu.RUnlock() // 确保函数退出时解锁。

	current := input // 当前层的输入。

	// 遍历每一层，计算其输出。
	for _, layer := range nn.layers {
		next := make([]float64, len(layer.bias)) // 下一层的输入（当前层的输出）。

		// 对下一层的每个神经元进行计算。
		for j := 0; j < len(layer.bias); j++ {
			sum := layer.bias[j] // 加上偏置。
			// 累加当前层所有神经元的加权和。
			for i := 0; i < len(current); i++ {
				sum += current[i] * layer.weights[i][j]
			}
			next[j] = nn.sigmoid(sum) // 应用激活函数。
		}

		current = next // 将当前层的输出作为下一层的输入。
	}

	return current // 返回最终输出层的输出。
}

// sigmoid 是一个常用的激活函数。
// 它将输入值压缩到 (0, 1) 的范围内，常用于输出层或隐藏层。
func (nn *NeuralNetwork) sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// Train 训练神经网络模型，使用简化的反向传播算法。
// 注意：此实现中的反向传播是高度简化的，仅用于演示目的。
// 实际的神经网络训练涉及更复杂的梯度计算、链式法则和优化器（如梯度下降）。
// 应用场景：复杂用户行为预测、推荐系统、图像识别等。
// points: 训练数据集的特征点切片。
// labels: 每个数据点对应的真实标签。
// learningRate: 学习率，控制每次权重更新的步长。
// iterations: 训练迭代次数（epochs）。
func (nn *NeuralNetwork) Train(points []*NNPoint, labels []int, learningRate float64, iterations int) {
	nn.mu.Lock()         // 训练过程需要加写锁。
	defer nn.mu.Unlock() // 确保函数退出时解锁。

	for iter := 0; iter < iterations; iter++ {
		for i, p := range points {
			// 前向传播：计算当前输入的网络输出。
			output := nn.Forward(p.Data)

			// 计算误差（这里是简单的差值，通常会使用损失函数）。
			target := float64(labels[i])
			error := target - output[0] // 假设输出层只有一个神经元。

			// 简化的反向传播：
			// 仅更新偏置项，且更新方式非常简单，未涉及权重更新和链式法则。
			// 实际中，错误会从输出层反向传播到每一层，更新所有权重和偏置。
			for j := 0; j < len(nn.layers); j++ {
				layer := nn.layers[j]
				for k := 0; k < len(layer.bias); k++ {
					// 偏置的更新 = 学习率 * 误差。
					// 这是一个非常简化的更新规则，实际中应使用误差关于偏置的梯度。
					layer.bias[k] += learningRate * error
				}
			}
		}
	}
}
