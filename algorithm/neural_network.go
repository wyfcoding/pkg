package algorithm

import (
	"log/slog"
	"math"
	"math/rand/v2"
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

// Train 训练神经网络模型，使用标准反向传播 (Backpropagation) 与随机梯度下降 (SGD)。
func (nn *NeuralNetwork) Train(points []*NNPoint, labels []int, learningRate float64, iterations int) {
	nn.mu.Lock()
	defer nn.mu.Unlock()

	for iter := range iterations {
		totalLoss := 0.0
		for i, p := range points {
			// 1. 前向传播
			output := nn.internalForward(p.Data)
			target := float64(labels[i])

			// 2. 计算输出层误差与 Delta
			lastLayerIdx := len(nn.layers) - 1
			outLayer := nn.layers[lastLayerIdx]
			deltas := make([][]float64, len(nn.layers))

			deltas[lastLayerIdx] = make([]float64, len(outLayer.output))
			for j := 0; j < len(outLayer.output); j++ {
				// 均方误差 (MSE) 导数: (output - target)
				err := output[j] - target
				totalLoss += 0.5 * err * err
				// 输出层 Delta = err * f'(net)
				deltas[lastLayerIdx][j] = err * nn.sigmoidDerivative(outLayer.output[j])
			}

			// 3. 隐藏层误差反向传播 (从后往前)
			for l := lastLayerIdx - 1; l >= 0; l-- {
				currLayer := nn.layers[l]
				nextLayer := nn.layers[l+1]
				deltas[l] = make([]float64, len(currLayer.output))

				for j := 0; j < len(currLayer.output); j++ {
					var err float64
					for k := 0; k < len(nextLayer.bias); k++ {
						// 误差项 = Sum(下一层 Delta * 下一层权重)
						err += deltas[l+1][k] * nextLayer.weights[j][k]
					}
					// 隐藏层 Delta = err * f'(net)
					deltas[l][j] = err * nn.sigmoidDerivative(currLayer.output[j])
				}
			}

			// 4. 执行参数更新 (权重 w 和 偏置 b)
			for l, layer := range nn.layers {
				for j := 0; j < len(layer.bias); j++ {
					// 梯度下降: w = w - lr * gradient
					// 其中 gradient = delta * input
					for k := 0; k < len(layer.input); k++ {
						layer.weights[k][j] -= learningRate * deltas[l][j] * layer.input[k]
					}
					layer.bias[j] -= learningRate * deltas[l][j]
				}
			}
		}

		// 顶级架构记录：可以在此处记录每轮迭代的 loss
		if iter%10 == 0 {
			slog.Debug("NN training epoch finished", "iteration", iter, "loss", totalLoss/float64(len(points)))
		}
	}
}
