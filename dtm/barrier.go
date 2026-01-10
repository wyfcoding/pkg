package dtm

import (
	"context"
	"errors"
	"log/slog"
	"reflect"

	"gorm.io/gorm"
)

var (
	// ErrInvalidBarrier 无效的事务屏障对象.
	ErrInvalidBarrier = errors.New("invalid dtm barrier")
	// ErrMethodNotFound 找不到指定的方法.
	ErrMethodNotFound = errors.New("method callwithgorm not found")
)

// CallWithGorm 是一项核心架构优化，旨在通过反射方式解耦 DTM 的具体实现.
func CallWithGorm(ctx context.Context, barrier any, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	val := reflect.ValueOf(barrier)
	if !val.IsValid() || val.IsNil() {
		slog.ErrorContext(ctx, "dtm barrier call failed: invalid or nil barrier object")

		return ErrInvalidBarrier
	}

	method := val.MethodByName("CallWithGorm")
	if !method.IsValid() {
		slog.ErrorContext(ctx, "dtm barrier call failed: target object missing CallWithGorm method")

		return ErrMethodNotFound
	}

	args := []reflect.Value{
		reflect.ValueOf(db),
		reflect.ValueOf(fn),
	}

	results := method.Call(args)

	if len(results) > 0 && !results[0].IsNil() {
		err, ok := results[0].Interface().(error)
		if ok {
			slog.WarnContext(ctx, "dtm barrier transaction reported error", "error", err)

			return err
		}
	}

	return nil
}
