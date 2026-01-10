// Package algorithm 提供了高性能算法实现。
package algorithm

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	randv2 "math/rand/v2"
	"sync"
)

// NNPoint 结构体代表数据集中的一个数据点。
type NNPoint struct {
	Data []float64
}

// NeuralNetwork 结构体实现了简单的全连接前馈神经网络。
type NeuralNetwork struct {
	layers []*Layer
	mu     sync.RWMutex
}

// Layer 结构体代表神经网络中的一个层。
type Layer struct {
	weights [][]float64
	bias    []float64
	input   []float64
	output  []float64
}

// NewNeuralNetwork 创建并返回一个新的 NeuralNetwork 实例。
func NewNeuralNetwork(layerSizes []int) (*NeuralNetwork, error) {
	if len(layerSizes) < 2 {
		return nil, errors.New("layerSizes must have at least 2 layers")
	}

	net := &NeuralNetwork{
		layers: make([]*Layer, len(layerSizes)-1),
		mu:     sync.RWMutex{},
	}

	for i := 0; i < len(layerSizes)-1; i++ {
		net.layers[i] = &Layer{
			weights: make([][]float64, layerSizes[i]),
			bias:    make([]float64, layerSizes[i+1]),
			input:   nil,
			output:  make([]float64, layerSizes[i+1]),
		}

		if err := net.initializeWeights(i, layerSizes[i], layerSizes[i+1]); err != nil {
			return nil, err
		}
	}

	return net, nil
}

func (nn *NeuralNetwork) initializeWeights(layerIdx, rows, cols int) error {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return err
	}

	randomSrc := randv2.New(randv2.NewPCG(binary.LittleEndian.Uint64(seed[:]), 0))
	stdDev := math.Sqrt(2.0 / float64(rows+cols))

	for i := 0; i < rows; i++ {
		nn.layers[layerIdx].weights[i] = make([]float64, cols)
		for j := 0; j < cols; j++ {
			nn.layers[layerIdx].weights[i][j] = (randomSrc.Float64()*2 - 1) * stdDev
		}
	}

	return nil
}

// Forward 执行前向传播。
func (nn *NeuralNetwork) Forward(input []float64) []float64 {
	nn.mu.RLock()
	defer nn.mu.RUnlock()

	return nn.internalForward(input)
}

func (nn *NeuralNetwork) internalForward(input []float64) []float64 {
	currentInput := input

	for _, layer := range nn.layers {
		layer.input = currentInput
		nextOutput := make([]float64, len(layer.bias))

		for j := 0; j < len(layer.bias); j++ {
			sum := layer.bias[j]
			for i := 0; i < len(currentInput); i++ {
				sum += currentInput[i] * layer.weights[i][j]
			}

			nextOutput[j] = nn.sigmoid(sum)
		}

		layer.output = nextOutput
		currentInput = nextOutput
	}

	return currentInput
}

func (nn *NeuralNetwork) sigmoid(val float64) float64 {
	return 1.0 / (1.0 + math.Exp(-val))
}

func (nn *NeuralNetwork) sigmoidDerivative(output float64) float64 {
	return output * (1.0 - output)
}

// Train 训练神经网络模型。
func (nn *NeuralNetwork) Train(points []*NNPoint, labels []int, learningRate float64, iterations int) {
	nn.mu.Lock()
	defer nn.mu.Unlock()

	for iter := 0; iter < iterations; iter++ {
		totalLoss := 0.0

		for i, point := range points {
			output := nn.internalForward(point.Data)
			target := float64(labels[i])

			deltas := nn.computeDeltas(output, target, &totalLoss)
			nn.updateParameters(deltas, learningRate)
		}

		if iter%10 == 0 {
			slog.Debug("NN training epoch finished", "iteration", iter, "loss", totalLoss/float64(len(points)))
		}
	}
}

func (nn *NeuralNetwork) computeDeltas(output []float64, target float64, totalLoss *float64) [][]float64 {
	numLayers := len(nn.layers)
	deltas := make([][]float64, numLayers)

	// 计算输出层 Delta。
	lastIdx := numLayers - 1
	outLayer := nn.layers[lastIdx]
	deltas[lastIdx] = make([]float64, len(outLayer.output))

	for j := 0; j < len(outLayer.output); j++ {
		errVal := output[j] - target
		*totalLoss += 0.5 * errVal * errVal
		deltas[lastIdx][j] = errVal * nn.sigmoidDerivative(outLayer.output[j])
	}

	// 隐藏层反向传播。
	for l := lastIdx - 1; l >= 0; l-- {
		currLayer := nn.layers[l]
		nextLayer := nn.layers[l+1]
		deltas[l] = make([]float64, len(currLayer.output))

		for j := 0; j < len(currLayer.output); j++ {
			var errAccum float64
			for k := 0; k < len(nextLayer.bias); k++ {
				errAccum += deltas[l+1][k] * nextLayer.weights[j][k]
			}

			deltas[l][j] = errAccum * nn.sigmoidDerivative(currLayer.output[j])
		}
	}

	return deltas
}

func (nn *NeuralNetwork) updateParameters(deltas [][]float64, learningRate float64) {
	for l, layer := range nn.layers {
		for j := 0; j < len(layer.bias); j++ {
			for k := 0; k < len(layer.input); k++ {
				layer.weights[k][j] -= learningRate * deltas[l][j] * layer.input[k]
			}

			layer.bias[j] -= learningRate * deltas[l][j]
		}
	}
}