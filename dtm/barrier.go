package dtm

import (
	"context"
	"errors"
	"log/slog"
	"reflect"

	"gorm.io/gorm"
)

// CallWithGorm 是一项核心架构优化，旨在通过反射方式解耦 DTM 的具体实现。
// 它将 any 类型的 barrier 对象映射至 DTM 的事务屏障逻辑，并自动开启 GORM 事务。
// 优势：避免了业务模块直接依赖 dtmcli 等底层库，减少了依赖冲突风险。
func CallWithGorm(ctx context.Context, barrier any, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	val := reflect.ValueOf(barrier)
	if !val.IsValid() || val.IsNil() {
		slog.ErrorContext(ctx, "dtm barrier call failed: invalid or nil barrier object")
		return errors.New("invalid dtm barrier")
	}

	method := val.MethodByName("CallWithGorm")
	if !method.IsValid() {
		slog.ErrorContext(ctx, "dtm barrier call failed: target object missing CallWithGorm method")
		return errors.New("method callwithgorm not found")
	}

	// 准备反射参数：db (GORM 实例), fn (业务回调函数)
	args := []reflect.Value{
		reflect.ValueOf(db),
		reflect.ValueOf(fn),
	}

	// 执行动态调用
	results := method.Call(args)

	// 统一处理执行结果中的错误
	if len(results) > 0 && !results[0].IsNil() {
		err := results[0].Interface().(error)
		slog.WarnContext(ctx, "dtm barrier transaction reported error", "error", err)
		return err
	}

	return nil
}
