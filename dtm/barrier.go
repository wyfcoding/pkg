package dtm

import (
	"context"
	"errors"
	"reflect"

	"gorm.io/gorm"
)

// CallWithGorm 辅助函数：将 interface{} 类型的 barrier 转为 *BranchBarrier 并执行 GORM 事务
// 这里的 barrier 通常由 dtmgrpc.BarrierFromGrpc(ctx) 获取
// 使用反射以避免直接引入 dtmcli 导致的依赖版本问题
func CallWithGorm(ctx context.Context, barrier any, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	val := reflect.ValueOf(barrier)
	if !val.IsValid() || val.IsNil() {
		return errors.New("barrier is nil")
	}

	method := val.MethodByName("CallWithGorm")
	if !method.IsValid() {
		return errors.New("barrier does not have CallWithGorm method")
	}

	// 准备参数: db, fn
	args := []reflect.Value{
		reflect.ValueOf(db),
		reflect.ValueOf(fn),
	}

	// 调用方法
	results := method.Call(args)

	// 处理返回值: error
	if len(results) > 0 && !results[0].IsNil() {
		return results[0].Interface().(error)
	}

	return nil
}
