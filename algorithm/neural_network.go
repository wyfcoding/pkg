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
	weights [][]float64 // 权重矩阵
	bias    []float64   // 偏置向量
	input   []float64   // 该层的输入 (用于反向传播)
	output  []float64   // 该层的输出 (用于反向传播)
}

// NewNeuralNetwork 创建并返回一个新的 NeuralNetwork 实例。
func NewNeuralNetwork(layerSizes []int) *NeuralNetwork {
	nn := &NeuralNetwork{
		layers: make([]*Layer, len(layerSizes)-1),
	}

	for i := 0; i < len(layerSizes)-1; i++ {
		nn.layers[i] = &Layer{
			weights: make([][]float64, layerSizes[i]),
			bias:    make([]float64, layerSizes[i+1]),
			output:  make([]float64, layerSizes[i+1]),
		}

		for j := 0; j < layerSizes[i]; j++ {
			nn.layers[i].weights[j] = make([]float64, layerSizes[i+1])
			for k := 0; k < layerSizes[i+1]; k++ {
				// Xavier/Glorot 初始化启发
				stdDev := math.Sqrt(2.0 / float64(layerSizes[i]+layerSizes[i+1]))
				nn.layers[i].weights[j][k] = (rand.Float64()*2 - 1) * stdDev
			}
		}
	}

	return nn
}

// Forward 执行前向传播并存储中间状态
func (nn *NeuralNetwork) Forward(input []float64) []float64 {
	// 注意：内部训练使用时不需要重新加锁，但为了保持公开 API 的安全，这里做逻辑拆分
	return nn.internalForward(input)
}

func (nn *NeuralNetwork) internalForward(input []float64) []float64 {
	current := input
	for _, layer := range nn.layers {
		layer.input = current
		next := make([]float64, len(layer.bias))
		for j := 0; j < len(layer.bias); j++ {
			sum := layer.bias[j]
			for i := 0; i < len(current); i++ {
				sum += current[i] * layer.weights[i][j]
			}
			next[j] = nn.sigmoid(sum)
		}
		layer.output = next
		current = next
	}
	return current
}

// sigmoid 是一个常用的激活函数。
func (nn *NeuralNetwork) sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// sigmoidDerivative 计算 Sigmoid 函数的导数: f'(x) = f(x) * (1 - f(x))
func (nn *NeuralNetwork) sigmoidDerivative(output float64) float64 {
	return output * (1.0 - output)
}

// Train 训练神经网络模型，使用真实的反向传播算法。
func (nn *NeuralNetwork) Train(points []*NNPoint, labels []int, learningRate float64, iterations int) {
	nn.mu.Lock()
	defer nn.mu.Unlock()

	for range iterations {
		for i, p := range points {
			// 1. 前向传播
			output := nn.internalForward(p.Data)

			// 2. 反向传播：计算 Deltas (误差项)
			deltas := make([][]float64, len(nn.layers))

			// 输出层 Delta: (target - output) * sigmoid'(output)
			target := float64(labels[i])
			lastLayerIdx := len(nn.layers) - 1
			outLayer := nn.layers[lastLayerIdx]
			deltas[lastLayerIdx] = make([]float64, len(outLayer.output))
			for j := 0; j < len(outLayer.output); j++ {
				// 这里假设输出层只有一个神经元 (针对二分类)
				err := target - output[j]
				deltas[lastLayerIdx][j] = err * nn.sigmoidDerivative(outLayer.output[j])
			}

			// 隐藏层 Deltas (从后往前算)
			for l := lastLayerIdx - 1; l >= 0; l-- {
				currLayer := nn.layers[l]
				nextLayer := nn.layers[l+1]
				deltas[l] = make([]float64, len(currLayer.output))
				for j := 0; j < len(currLayer.output); j++ {
					var err float64
					for k := 0; k < len(nextLayer.bias); k++ {
						// 误差反向传导：使用下一层的权重
						err += deltas[l+1][k] * nextLayer.weights[j][k]
					}
					deltas[l][j] = err * nn.sigmoidDerivative(currLayer.output[j])
				}
			}

			// 3. 更新权重和偏置
			for l, layer := range nn.layers {
				for j := 0; j < len(layer.bias); j++ {
					// 更新权重: w = w + lr * delta * input
					for k := 0; k < len(layer.input); k++ {
						layer.weights[k][j] += learningRate * deltas[l][j] * layer.input[k]
					}
					// 更新偏置: b = b + lr * delta
					layer.bias[j] += learningRate * deltas[l][j]
				}
			}
		}
	}
}
