package algorithm

import (
	"math"
	"sync"
)

// NaiveBayes 结构体实现了朴素贝叶斯分类器。
// 优化：使用倒排索引结构（Word -> []Prob）加速预测，减少 Map 查找次数。
type NaiveBayes struct {
	vocab          map[string][]float64 // 词汇表：单词 -> [类别1条件对数概率, 类别2条件对数概率...]。
	mu             sync.RWMutex
	classNames     []string  // 类别名称列表 (Index -> Name)。
	classLogPriors []float64 // 类别先验对数概率 (Index -> LogProb)。
}

// NewNaiveBayes 创建并返回一个新的 NaiveBayes 分类器实例。
func NewNaiveBayes() *NaiveBayes {
	return &NaiveBayes{
		classNames:     make([]string, 0),
		classLogPriors: make([]float64, 0),
		vocab:          make(map[string][]float64),
	}
}

// Train 训练朴素贝叶斯分类器模型。
func (nb *NaiveBayes) Train(documents [][]string, labels []string) {
	nb.mu.Lock()
	defer nb.mu.Unlock()

	totalDocs := len(documents)
	if totalDocs == 0 {
		return
	}

	// 临时统计结果。
	classCounts := make(map[string]int)
	featureCounts := make(map[string]map[string]int)
	globalVocab := make(map[string]bool)

	// 1. 统计频率。
	for i, doc := range documents {
		label := labels[i]
		classCounts[label]++
		if featureCounts[label] == nil {
			featureCounts[label] = make(map[string]int)
		}
		for _, word := range doc {
			featureCounts[label][word]++
			globalVocab[word] = true
		}
	}

	// 2. 构建类别索引。
	nb.classNames = make([]string, 0, len(classCounts))
	nb.classLogPriors = make([]float64, 0, len(classCounts))
	for cls := range classCounts {
		nb.classNames = append(nb.classNames, cls)
	}

	// 3. 计算先验概率和条件概率 (Log Space)。
	vocabSize := len(globalVocab)
	nb.vocab = make(map[string][]float64, vocabSize)
	numClasses := len(nb.classNames)

	// 预初始化所有单词的概率数组。
	for word := range globalVocab {
		nb.vocab[word] = make([]float64, numClasses)
	}

	for i, cls := range nb.classNames {
		// 计算先验 log P(C)。
		count := classCounts[cls]
		nb.classLogPriors = append(nb.classLogPriors, math.Log(float64(count)/float64(totalDocs)))

		// 计算该类别的总词数 (用于分母)。
		totalWordsInClass := 0
		for _, c := range featureCounts[cls] {
			totalWordsInClass += c
		}
		denominator := float64(totalWordsInClass + vocabSize) // 拉普拉斯平滑。

		// 计算每个词的条件概率 log P(W|C)。
		for word := range globalVocab {
			wordCount := featureCounts[cls][word]
			// 分子 + 1 平滑。
			prob := float64(wordCount+1) / denominator
			nb.vocab[word][i] = math.Log(prob)
		}
	}
}

// Predict 预测给定文档的类别标签。
func (nb *NaiveBayes) Predict(document []string) string {
	label, _ := nb.PredictWithConfidence(document)
	return label
}

// PredictWithConfidence 预测给定文档的类别标签并返回置信度。
// 优化后复杂度：O(Words + Classes) 而非 O(Words * Classes)。
func (nb *NaiveBayes) PredictWithConfidence(document []string) (label string, confidence float64) {
	nb.mu.RLock()
	defer nb.mu.RUnlock()

	numClasses := len(nb.classNames)
	if numClasses == 0 {
		return "", 0.0
	}

	// 初始化得分为先验概率。
	scores := make([]float64, numClasses)
	copy(scores, nb.classLogPriors)

	// 累加词语的条件概率。
	for _, word := range document {
		if probs, ok := nb.vocab[word]; ok {
			for i, p := range probs {
				scores[i] += p
			}
		}
		// 如果单词不在词汇表中，通常忽略 (或应用更复杂的平滑，这里简化为忽略)。
	}

	// 寻找最大值。
	maxScore := math.Inf(-1)
	maxIdx := -1
	for i, s := range scores {
		if s > maxScore {
			maxScore = s
			maxIdx = i
		}
	}

	if maxIdx == -1 {
		return "", 0.0
	}

	// Softmax 归一化计算置信度。
	sumExp := 0.0
	for _, s := range scores {
		sumExp += math.Exp(s - maxScore)
	}
	confidence = 1.0 / sumExp

	return nb.classNames[maxIdx], confidence
}
