package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/xerrors"
	"gorm.io/gorm"
)

// AnalyticsWriter 提供了向 ClickHouse 批量写入数据的能力.
type AnalyticsWriter struct {
	db     *gorm.DB
	logger *logging.Logger
}

// NewAnalyticsWriter 创建一个新的 AnalyticsWriter.
func NewAnalyticsWriter(db *gorm.DB, logger *logging.Logger) *AnalyticsWriter {
	return &AnalyticsWriter{
		db:     db,
		logger: logger,
	}
}

// BatchInsert 批量插入数据到指定表.
func (w *AnalyticsWriter) BatchInsert(ctx context.Context, table string, data any) error {
	start := time.Now()
	err := w.db.WithContext(ctx).Table(table).Create(data).Error
	duration := time.Since(start)

	if err != nil {
		w.logger.Error("failed to batch insert into ClickHouse",
			slog.String("table", table),
			slog.String("duration", duration.String()),
			slog.Any("error", err),
		)
		return xerrors.WrapInternal(err, fmt.Sprintf("failed to insert data into %s", table))
	}

	w.logger.Info("successfully inserted data into ClickHouse",
		slog.String("table", table),
		slog.String("duration", duration.String()),
	)
	return nil
}
