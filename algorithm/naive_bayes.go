package algorithm

import (
	"math"
	"sync"
)

// NaiveBayes 结构体实现了朴素贝叶斯分类器。
// 朴素贝叶斯是一种基于贝叶斯定理的概率分类器，其“朴素”之处在于假设所有特征之间相互独立。
// 尽管有这一强假设，朴素贝叶斯在文本分类等许多实际应用中表现出色。
type NaiveBayes struct {
	classes      map[string]float64            // 存储每个类别的先验概率 P(C)。
	features     map[string]map[string]float64 // 存储每个特征在给定类别下的条件概率 P(F | C)。
	classCount   map[string]int                // 训练集中每个类别的文档数量。
	featureCount map[string]map[string]int     // 训练集中在给定类别下每个特征（词）出现的次数。
	mu           sync.RWMutex                  // 读写锁，用于保护分类器参数的并发访问。
}

// NewNaiveBayes 创建并返回一个新的 NaiveBayes 分类器实例。
func NewNaiveBayes() *NaiveBayes {
	return &NaiveBayes{
		classes:      make(map[string]float64),
		features:     make(map[string]map[string]float64),
		classCount:   make(map[string]int),
		featureCount: make(map[string]map[string]int),
	}
}

// Train 训练朴素贝叶斯分类器模型。
// 它根据给定的文档和对应的标签，计算各类别和特征的概率。
// 应用场景：商品评论分类、内容审核、垃圾邮件识别等。
// documents: 训练数据集，每个文档是一个词语（特征）切片。
// labels: 每个文档对应的类别标签。
func (nb *NaiveBayes) Train(documents [][]string, labels []string) {
	nb.mu.Lock()         // 训练过程需要加写锁。
	defer nb.mu.Unlock() // 确保函数退出时解锁。

	totalDocs := len(documents) // 训练文档总数。

	// 步骤1: 计算每个类别的文档数量，进而计算类别的先验概率 P(C)。
	for _, label := range labels {
		nb.classCount[label]++
	}

	for label, count := range nb.classCount {
		nb.classes[label] = float64(count) / float64(totalDocs)
	}

	// 步骤2: 统计每个特征（词）在每个类别中出现的次数。
	for i, doc := range documents {
		label := labels[i]

		if nb.featureCount[label] == nil {
			nb.featureCount[label] = make(map[string]int)
		}

		for _, word := range doc {
			nb.featureCount[label][word]++
		}
	}

	// 步骤3: 根据统计结果，计算每个特征在给定类别下的条件概率 P(F | C)。
	// 为了避免零概率问题（即某个词在训练集中未出现，导致其概率为0），这里使用了拉普拉斯平滑。
	for label := range nb.classCount {
		nb.features[label] = make(map[string]float64)

		totalWords := 0 // 当前类别下所有词的总数。
		for _, count := range nb.featureCount[label] {
			totalWords += count
		}
		// 词汇表大小，用于拉普拉斯平滑，这里简化的使用了当前类别下不同词的数量。
		// 更严谨的拉普拉斯平滑应使用整个训练集中的所有不同词的数量（词汇表大小）。
		vocabularySize := len(nb.featureCount[label])

		for word, count := range nb.featureCount[label] {
			// 拉普拉斯平滑公式: P(F|C) = (Count(F,C) + 1) / (Count(C) + VocabularySize)
			nb.features[label][word] = float64(count+1) / float64(totalWords+vocabularySize)
		}
	}
}

// Predict 预测给定文档的类别标签。
// document: 待分类的文档，表示为一个词语切片。
// 返回文档最可能所属的类别标签。
func (nb *NaiveBayes) Predict(document []string) string {
	label, _ := nb.PredictWithConfidence(document)
	return label
}

// PredictWithConfidence 预测给定文档的类别标签并返回置信度。
// document: 待分类的文档。
// 返回预测的标签和 0.0 到 1.0 之间的置信度。
func (nb *NaiveBayes) PredictWithConfidence(document []string) (string, float64) {
	nb.mu.RLock()
	defer nb.mu.RUnlock()

	if len(nb.classes) == 0 {
		return "", 0.0
	}

	scores := make(map[string]float64)
	maxProb := math.Inf(-1)
	maxLabel := ""

	// 1. 计算每个类别的对数得分
	for label, classProb := range nb.classes {
		prob := math.Log(classProb)
		for _, word := range document {
			if wordProb, exists := nb.features[label][word]; exists {
				prob += math.Log(wordProb)
			} else {
				vocabularySize := len(nb.featureCount[label])
				prob += math.Log(1.0 / float64(nb.classCount[label]+vocabularySize+1))
			}
		}
		scores[label] = prob
		if prob > maxProb {
			maxProb = prob
			maxLabel = label
		}
	}

	// 2. 将对数得分转换为归一化概率（置信度）
	// 为了防止数值溢出，使用 Softmax 技巧：P(i) = exp(s_i - max_s) / sum(exp(s_j - max_s))
	sumExp := 0.0
	for _, score := range scores {
		sumExp += math.Exp(score - maxProb)
	}

	confidence := 1.0 / sumExp // 因为 P(max) = exp(max_s - max_s) / sumExp = 1 / sumExp

	return maxLabel, confidence
}
