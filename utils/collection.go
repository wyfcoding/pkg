// Package utils 提供了通用的集合处理工具.
package utils

// Filter 对切片进行过滤，返回符合条件的元素.
func Filter[T any](items []T, fn func(T) bool) []T {
	var result []T
	for _, item := range items {
		if fn(item) {
			result = append(result, item)
		}
	}
	return result
}

// Map 对切片进行转换，返回转换后的新切片.
func Map[T any, R any](items []T, fn func(T) R) []R {
	result := make([]R, len(items))
	for i, item := range items {
		result[i] = fn(item)
	}
	return result
}

// Find 查找第一个符合条件的元素，如果没找到返回零值和 false.
func Find[T any](items []T, fn func(T) bool) (T, bool) {
	for _, item := range items {
		if fn(item) {
			return item, true
		}
	}
	var zero T
	return zero, false
}

// Contains 判断切片是否包含某个元素.
func Contains[T comparable](items []T, target T) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// Unique 返回去重后的切片.
func Unique[T comparable](items []T) []T {
	seen := make(map[T]struct{})
	var result []T
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// ToMap 将切片转换为 Map.
func ToMap[T any, K comparable, V any](items []T, keyFn func(T) K, valFn func(T) V) map[K]V {
	result := make(map[K]V, len(items))
	for _, item := range items {
		result[keyFn(item)] = valFn(item)
	}
	return result
}

// ToSet 将切片转换为 Map Set (用于 O(1) 查找).
func ToSet[T comparable](items []T) map[T]struct{} {
	result := make(map[T]struct{}, len(items))
	for _, item := range items {
		result[item] = struct{}{}
	}
	return result
}

// Chunk 将切片按指定大小分块，常用于批量处理逻辑.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 {
		return [][]T{items}
	}
	var chunks [][]T
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

// Flatten 将二维切片平铺为一维切片.
func Flatten[T any](items [][]T) []T {
	var result []T
	for _, sub := range items {
		result = append(result, sub...)
	}
	return result
}

// GroupBy 将切片按指定规则分组.
func GroupBy[T any, K comparable](items []T, keyFn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, item := range items {
		key := keyFn(item)
		result[key] = append(result[key], item)
	}
	return result
}

// Reduce 对切片进行归约处理.
func Reduce[T any, R any](items []T, initial R, fn func(R, T) R) R {
	result := initial
	for _, item := range items {
		result = fn(result, item)
	}
	return result
}
