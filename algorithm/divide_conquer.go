package algorithm

import (
	"sync"
)

// DivideAndConquerProcessor 是一个实现了分治算法的处理器。
// 它主要用于处理大规模数据集的排序和分析任务。
type DivideAndConquerProcessor struct {
	data []int64      // 待处理的整数切片数据。
	mu   sync.RWMutex // 读写锁，用于保护数据的并发访问。
}

// NewDivideAndConquerProcessor 创建并返回一个新的 DivideAndConquerProcessor 实例。
// data: 需要进行处理的原始数据切片。
func NewDivideAndConquerProcessor(data []int64) *DivideAndConquerProcessor {
	return &DivideAndConquerProcessor{
		data: data,
	}
}

// MergeSort 对处理器中的数据执行归并排序。
// 这是一种经典的分治算法，时间复杂度为 O(n log n)。
// 优化：复用辅助数组，避免递归过程中的内存分配。
func (dcp *DivideAndConquerProcessor) MergeSort() []int64 {
	dcp.mu.Lock()
	defer dcp.mu.Unlock()

	if len(dcp.data) <= 1 {
		return dcp.data
	}

	result := make([]int64, len(dcp.data))
	copy(result, dcp.data)

	// 预分配辅助数组。
	temp := make([]int64, len(dcp.data))
	dcp.mergeSort(result, temp, 0, len(result)-1)

	return result
}

// mergeSort 是归并排序的递归实现。
func (dcp *DivideAndConquerProcessor) mergeSort(arr, temp []int64, left, right int) {
	if left < right {
		mid := (left + right) / 2
		dcp.mergeSort(arr, temp, left, mid)
		dcp.mergeSort(arr, temp, mid+1, right)
		dcp.merge(arr, temp, left, mid, right)
	}
}

// merge 合并两个有序子数组。
func (dcp *DivideAndConquerProcessor) merge(arr, temp []int64, left, mid, right int) {
	// 将数据复制到辅助数组。
	copy(temp[left:right+1], arr[left:right+1])

	idxI := left    // 左半部分指针。
	idxJ := mid + 1 // 右半部分指针。
	idxK := left    // 原数组指针。

	for idxI <= mid && idxJ <= right {
		if temp[idxI] <= temp[idxJ] {
			arr[idxK] = temp[idxI]
			idxI++
		} else {
			arr[idxK] = temp[idxJ]
			idxJ++
		}
		idxK++
	}

	// 复制左半部分剩余元素。
	// 右半部分剩余元素不需要复制，因为它们已经在原数组的正确位置。
	for idxI <= mid {
		arr[idxK] = temp[idxI]
		idxI++
		idxK++
	}
}

// CountInversions 计算处理器中数据的逆序对数量。
// 逆序对是指在一个序列中，a[i] > a[j] 且 i < j 的一对元素。
// 此算法利用归并排序的思想，时间复杂度为 O(n log n)，可用于分析数据的异常度或离散程度。
func (dcp *DivideAndConquerProcessor) CountInversions() int64 {
	dcp.mu.Lock()         // 加写锁，以确保在计算过程中数据不会被修改。
	defer dcp.mu.Unlock() // 确保函数退出时解锁。

	if len(dcp.data) <= 1 {
		return 0 // 0 个或 1 个元素的数组没有逆序对。
	}

	arr := make([]int64, len(dcp.data)) // 创建一个副本进行处理。
	copy(arr, dcp.data)
	_, count := dcp.mergeSortCount(arr, 0, len(arr)-1) // 调用递归函数计算逆序对。

	return count
}

// mergeSortCount 是归并排序和计算逆序对的结合实现。
// 它在合并两个有序子数组时，计算跨越两个子数组的逆序对。
// arr: 待处理的切片。
// left, right: 当前子数组的起始和结束索引。
// 返回：处理后的切片（已排序）和逆序对的数量。
func (dcp *DivideAndConquerProcessor) mergeSortCount(arr []int64, left, right int) (sortedArr []int64, invCount int64) {
	if left >= right {
		return arr, 0 // 单个元素或空数组没有逆序对。
	}

	mid := (left + right) / 2
	_, leftCount := dcp.mergeSortCount(arr, left, mid)     // 递归计算左半部分的逆序对。
	_, rightCount := dcp.mergeSortCount(arr, mid+1, right) // 递归计算右半部分的逆序对。

	mergeCount := dcp.mergeCount(arr, left, mid, right) // 计算合并时产生的逆序对。

	return arr, leftCount + rightCount + mergeCount // 返回总的逆序对数量。
}

// mergeCount 合并两个有序子数组，并计算合并过程中产生的逆序对数量。
// arr: 包含两个有序子数组的切片。
// left, mid, right: 定义两个子数组的边界。
// 返回：合并时产生的逆序对数量。
func (dcp *DivideAndConquerProcessor) mergeCount(arr []int64, left, mid, right int) int64 {
	// 创建临时切片存储左右两部分。
	leftArr := make([]int64, mid-left+1)
	rightArr := make([]int64, right-mid)

	copy(leftArr, arr[left:mid+1])
	copy(rightArr, arr[mid+1:right+1])

	idxI, idxJ, idxK := 0, 0, left
	var count int64 // 记录逆序对数量。

	// 比较左右两个子数组的元素，并将较小的元素放回原数组。
	for idxI < len(leftArr) && idxJ < len(rightArr) {
		if leftArr[idxI] <= rightArr[idxJ] {
			arr[idxK] = leftArr[idxI]
			idxI++
		} else {
			arr[idxK] = rightArr[idxJ]
			// 如果 leftArr[i] > rightArr[j]，则 leftArr 中剩余的所有元素。
			// (从 idxI 到 len(leftArr)-1) 都比 rightArr[idxJ] 大，形成逆序对。
			count += int64(len(leftArr) - idxI)
			idxJ++
		}
		idxK++
	}

	// 将剩余的左半部分元素拷贝回原数组。
	for idxI < len(leftArr) {
		arr[idxK] = leftArr[idxI]
		idxI++
		idxK++
	}

	// 将剩余的右半部分元素拷贝回原数组。
	for idxJ < len(rightArr) {
		arr[idxK] = rightArr[idxJ]
		idxJ++
		idxK++
	}

	return count
}
