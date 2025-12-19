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
// 这是一种经典的分治算法，时间复杂度为 O(n log n)，适用于大规模数据的排序，例如订单或库存列表的排序。
// 返回一个包含排序后数据的新切片，原始数据不会被修改。
func (dcp *DivideAndConquerProcessor) MergeSort() []int64 {
	dcp.mu.Lock()         // 加写锁，以确保在排序过程中数据不会被修改。
	defer dcp.mu.Unlock() // 确保函数退出时解锁。

	if len(dcp.data) <= 1 {
		return dcp.data // 1个或0个元素的数组已经是排序好的。
	}

	result := make([]int64, len(dcp.data)) // 创建一个副本进行排序，不修改原始数据。
	copy(result, dcp.data)
	dcp.mergeSort(result, 0, len(result)-1) // 调用递归的归并排序函数。
	return result
}

// mergeSort 是归并排序的递归实现。
// arr: 待排序的切片。
// left, right: 当前子数组的起始和结束索引。
func (dcp *DivideAndConquerProcessor) mergeSort(arr []int64, left, right int) {
	if left < right {
		mid := (left + right) / 2        // 计算中间索引。
		dcp.mergeSort(arr, left, mid)    // 递归排序左半部分。
		dcp.mergeSort(arr, mid+1, right) // 递归排序右半部分。
		dcp.merge(arr, left, mid, right) // 合并两个已排序的子数组。
	}
}

// merge 合并两个有序子数组。
// arr: 包含两个有序子数组的切片。
// left, mid, right: 定义两个子数组的边界。
func (dcp *DivideAndConquerProcessor) merge(arr []int64, left, mid, right int) {
	// 创建临时切片存储左右两部分。
	leftArr := make([]int64, mid-left+1)
	rightArr := make([]int64, right-mid)

	copy(leftArr, arr[left:mid+1])
	copy(rightArr, arr[mid+1:right+1])

	i, j, k := 0, 0, left // i: leftArr索引, j: rightArr索引, k: arr索引。

	// 比较左右两个子数组的元素，并将较小的元素放回原数组。
	for i < len(leftArr) && j < len(rightArr) {
		if leftArr[i] <= rightArr[j] {
			arr[k] = leftArr[i]
			i++
		} else {
			arr[k] = rightArr[j]
			j++
		}
		k++
	}

	// 将剩余的左半部分元素拷贝回原数组。
	for i < len(leftArr) {
		arr[k] = leftArr[i]
		i++
		k++
	}

	// 将剩余的右半部分元素拷贝回原数组。
	for j < len(rightArr) {
		arr[k] = rightArr[j]
		j++
		k++
	}
}

// CountInversions 计算处理器中数据的逆序对数量。
// 逆序对是指在一个序列中，a[i] > a[j] 且 i < j 的一对元素。
// 此算法利用归并排序的思想，时间复杂度为 O(n log n)，可用于分析数据的异常度或离散程度。
func (dcp *DivideAndConquerProcessor) CountInversions() int64 {
	dcp.mu.Lock()         // 加写锁，以确保在计算过程中数据不会被修改。
	defer dcp.mu.Unlock() // 确保函数退出时解锁。

	if len(dcp.data) <= 1 {
		return 0 // 0个或1个元素的数组没有逆序对。
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
func (dcp *DivideAndConquerProcessor) mergeSortCount(arr []int64, left, right int) ([]int64, int64) {
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

	i, j, k := 0, 0, left
	var count int64 = 0 // 记录逆序对数量。

	// 比较左右两个子数组的元素，并将较小的元素放回原数组。
	for i < len(leftArr) && j < len(rightArr) {
		if leftArr[i] <= rightArr[j] {
			arr[k] = leftArr[i]
			i++
		} else {
			arr[k] = rightArr[j]
			// 如果 leftArr[i] > rightArr[j]，则 leftArr 中剩余的所有元素
			// (从 i 到 len(leftArr)-1) 都比 rightArr[j] 大，形成逆序对。
			count += int64(len(leftArr) - i)
			j++
		}
		k++
	}

	// 将剩余的左半部分元素拷贝回原数组。
	for i < len(leftArr) {
		arr[k] = leftArr[i]
		i++
		k++
	}

	// 将剩余的右半部分元素拷贝回原数组。
	for j < len(rightArr) {
		arr[k] = rightArr[j]
		j++
		k++
	}

	return count
}
