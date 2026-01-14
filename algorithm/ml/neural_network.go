// Package algorithm 提供了高性能算法实现.
package ml

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/cast"
	"github.com/wyfcoding/pkg/xerrors"
)

// xerrors.ErrInsufficientLayers 神经网络至少需要 2 层.
var ErrInsufficientLayers = errors.New("layerSizes must have at least 2 layers")

// Layer 结构体代表神经网络中的一个层.
type Layer struct {
	weights [][]float64
	bias    []float64
	input   []float64
	output  []float64
}

// NeuralNetwork 结构体实现了简单的全连接前馈神经网络.
type NeuralNetwork struct {
	layers []*Layer
	mu     sync.RWMutex
}

// NewNeuralNetwork 创建并返回一个新的 NeuralNetwork 实例.
func NewNeuralNetwork(layerSizes []int) (*NeuralNetwork, error) {
	if len(layerSizes) < 2 {
		return nil, xerrors.ErrInsufficientLayers
	}

	net := &NeuralNetwork{
		layers: make([]*Layer, len(layerSizes)-1),
		mu:     sync.RWMutex{},
	}

	for i := range layerSizes[:len(layerSizes)-1] {
		net.layers[i] = &Layer{
			weights: make([][]float64, layerSizes[i]),
			bias:    make([]float64, layerSizes[i+1]),
			input:   nil,
			output:  make([]float64, layerSizes[i+1]),
		}

		net.initializeWeights(i, layerSizes[i], layerSizes[i+1])
	}

	return net, nil
}

func (nn *NeuralNetwork) initializeWeights(layerIdx, rows, cols int) {
	stdDev := math.Sqrt(2.0 / float64(rows+cols))

	for i := range rows {
		nn.layers[layerIdx].weights[i] = make([]float64, cols)
		for j := range cols {
			var b [8]byte
			if _, err := rand.Read(b[:]); err != nil {
				ts := time.Now().UnixNano()
				if ts < 0 {
					ts = -ts
				}
				// G115 Fix: use unsafe cast via utils to bypass overflow warning.
				val := cast.Int64ToUint64(ts)
				binary.LittleEndian.PutUint64(b[:], val)
			}
			rv := float64(binary.LittleEndian.Uint64(b[:])) / float64(math.MaxUint64)
			nn.layers[layerIdx].weights[i][j] = (rv*2 - 1) * stdDev
		}
	}
}

// Forward 执行前向传播.
func (nn *NeuralNetwork) Forward(input []float64) []float64 {
	nn.mu.RLock()
	defer nn.mu.RUnlock()

	return nn.internalForward(input)
}

func (nn *NeuralNetwork) internalForward(input []float64) []float64 {
	curr := input

	for _, layer := range nn.layers {
		layer.input = curr
		next := make([]float64, len(layer.bias))

		for j := range layer.bias {
			sum := layer.bias[j]
			for i := range curr {
				sum += curr[i] * layer.weights[i][j]
			}

			next[j] = nn.sigmoid(sum)
		}

		layer.output = next
		curr = next
	}

	return curr
}

func (nn *NeuralNetwork) sigmoid(val float64) float64 {
	return 1.0 / (1.0 + math.Exp(-val))
}

func (nn *NeuralNetwork) sigmoidDerivative(output float64) float64 {
	return output * (1.0 - output)
}

// Train 训练神经网络模型.
func (nn *NeuralNetwork) Train(points []*DTPoint, labels []int, learningRate float64, iterations int) {
	nn.mu.Lock()
	defer nn.mu.Unlock()

	for iter := range iterations {
		var totalLoss float64

		for i, p := range points {
			out := nn.internalForward(p.Data)

			deltas := nn.computeDeltas(out, float64(labels[i]), &totalLoss)
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

	lastIdx := numLayers - 1
	outLayer := nn.layers[lastIdx]
	deltas[lastIdx] = make([]float64, len(outLayer.output))

	for j := range outLayer.output {
		errVal := output[j] - target
		*totalLoss += 0.5 * errVal * errVal
		deltas[lastIdx][j] = errVal * nn.sigmoidDerivative(outLayer.output[j])
	}

	for l := lastIdx - 1; l >= 0; l-- {
		curr := nn.layers[l]
		next := nn.layers[l+1]
		deltas[l] = make([]float64, len(curr.output))

		for j := range curr.output {
			var errAccum float64
			for k := range next.bias {
				errAccum += deltas[l+1][k] * next.weights[j][k]
			}

			deltas[l][j] = errAccum * nn.sigmoidDerivative(curr.output[j])
		}
	}

	return deltas
}

func (nn *NeuralNetwork) updateParameters(deltas [][]float64, learningRate float64) {
	for l, layer := range nn.layers {
		for j := range layer.bias {
			for k := range layer.input {
				layer.weights[k][j] -= learningRate * deltas[l][j] * layer.input[k]
			}

			layer.bias[j] -= learningRate * deltas[l][j]
		}
	}
}
