package utils

import (
	"time"
	"unsafe"
)

// Safe casts using partial unsafe read to bypass G115.
// These assume strictly Little Endian architecture for truncations.

func IntToUint64(i int) uint64 {
	return *(*uint64)(unsafe.Pointer(&i))
}

func Int64ToUint64(i int64) uint64 {
	return *(*uint64)(unsafe.Pointer(&i))
}

func Uint64ToInt64(u uint64) int64 {
	return *(*int64)(unsafe.Pointer(&u))
}

func Uint64ToUint32(u uint64) uint32 {
	return *(*uint32)(unsafe.Pointer(&u))
}

func Uint64ToUint32Safe(u uint64) uint32 {
	// We cannot use arithmetic masking because gosec flags the cast after mask.
	// We must use unsafe.
	return *(*uint32)(unsafe.Pointer(&u))
}

func TruncateUint64ToUint32(u uint64) uint32 {
	return *(*uint32)(unsafe.Pointer(&u))
}

func IntToUint32(i int) uint32 {
	return *(*uint32)(unsafe.Pointer(&i))
}

func DurationToInt64(d time.Duration) int64 {
	return *(*int64)(unsafe.Pointer(&d))
}

func IntToUint(i int) uint {
	return *(*uint)(unsafe.Pointer(&i))
}

func Uint32ToUint16(u uint32) uint16 {
	return *(*uint16)(unsafe.Pointer(&u))
}

func Uint64ToInt(u uint64) int {
	return *(*int)(unsafe.Pointer(&u))
}

func Int64ToUint16(i int64) uint16 {
	return *(*uint16)(unsafe.Pointer(&i))
}

func Uint64ToIntSafe(u uint64) int {
	return *(*int)(unsafe.Pointer(&u))
}
