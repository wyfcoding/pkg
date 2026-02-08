// Package audit 提供审计事件的统一模型与写入器接口。
// 生成摘要:
// 1) 新增基于消息总线的审计事件写入器。
// 2) 自动注入请求上下文元数据，便于跨服务追踪。
// 假设:
// 1) 消息总线已初始化并支持领域事件发布。
package audit

import (
	"context"
	"errors"

	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/eventsourcing"
	"github.com/wyfcoding/pkg/messagequeue"
	"github.com/wyfcoding/pkg/tracing"
)

var (
	// ErrBusUnavailable 表示审计消息总线不可用。
	ErrBusUnavailable = errors.New("audit event bus unavailable")
)

const defaultAuditEventType = "audit.event"

// EventBusWriter 将审计事件发布到统一消息总线。
type EventBusWriter struct {
	Bus       messagequeue.EventBus
	EventType string
}

// NewEventBusWriter 创建审计事件总线写入器。
func NewEventBusWriter(bus messagequeue.EventBus) *EventBusWriter {
	return &EventBusWriter{Bus: bus}
}

// Write 发布审计事件到消息总线。
func (w *EventBusWriter) Write(ctx context.Context, event Event) error {
	if w == nil || w.Bus == nil {
		return ErrBusUnavailable
	}

	eventType := w.EventType
	if eventType == "" {
		eventType = defaultAuditEventType
	}

	aggregateID := chooseAggregateID(event)
	metadata := eventsourcing.Metadata{
		CorrelationID: contextx.GetRequestID(ctx),
		CausationID:   event.Action,
		UserID:        contextx.GetUserID(ctx),
		TraceID:       tracing.GetTraceID(ctx),
		Extra: map[string]string{
			"tenant_id": event.TenantID,
			"resource":  event.Resource,
		},
	}

	base := eventsourcing.NewBaseEventWithMetadata(eventType, aggregateID, 0, metadata)
	base.Data = event

	return w.Bus.Publish(ctx, &base)
}

func chooseAggregateID(event Event) string {
	if event.ResourceID != "" {
		return event.ResourceID
	}
	if event.ActorID != "" {
		return event.ActorID
	}
	if event.Action != "" {
		return event.Action
	}
	return "audit"
}
