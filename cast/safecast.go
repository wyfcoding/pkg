package cast

import (
	"time"
	"unsafe"
)

// As 是一个极致高性能的泛型转换函数，通过 unsafe 直接读取内存。
// 警告：仅限用于已知物理布局兼容的类型转换（如 int -> uint64 的截断或位读取）。
func As[T any, F any](from F) T {
	return *(*T)(unsafe.Pointer(&from))
}

// 以下保留常用别名以提高代码可读性，但底层统一调用泛型 As.

func IntToUint64(i int) uint64       { return As[uint64](i) }
func Int64ToUint64(i int64) uint64   { return As[uint64](i) }
func Uint64ToInt64(u uint64) int64   { return As[int64](u) }
func Uint64ToUint32(u uint64) uint32 { return As[uint32](u) }
func IntToUint32(i int) uint32       { return As[uint32](i) }
func IntToUint(i int) uint           { return As[uint](i) }
func Uint32ToUint16(u uint32) uint16 { return As[uint16](u) }
func Uint64ToInt(u uint64) int       { return As[int](u) }
func Uint64ToIntSafe(u uint64) int   { return As[int](u) }
func Int64ToUint16(i int64) uint16   { return As[uint16](i) }

func DurationToInt64(d time.Duration) int64 { return As[int64](d) }
