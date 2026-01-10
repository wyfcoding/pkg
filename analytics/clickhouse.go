package analytics

import (
	"context"
	"log/slog"
	"time"

	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/xerrors"
	"gorm.io/gorm"
)

// Writer 提供了向 ClickHouse 批量写入数据的能力.
type Writer struct {
	db     *gorm.DB
	logger *logging.Logger
}

// NewWriter 创建一个新的 Writer.
func NewWriter(db *gorm.DB, logger *logging.Logger) *Writer {
	return &Writer{
		db:     db,
		logger: logger,
	}
}

// BatchInsert 批量插入数据到指定表.
func (w *Writer) BatchInsert(ctx context.Context, table string, data any) error {
	start := time.Now()
	err := w.db.WithContext(ctx).Table(table).Create(data).Error
	duration := time.Since(start)

	if err != nil {
		w.logger.Error("failed to batch insert into ClickHouse",
			slog.String("table", table),
			slog.String("duration", duration.String()),
			slog.Any("error", err),
		)
		return xerrors.WrapInternal(err, "failed to insert data into "+table)
	}

	w.logger.Info("successfully inserted data into ClickHouse",
		slog.String("table", table),
		slog.String("duration", duration.String()),
	)
	return nil
}
