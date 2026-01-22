package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"
)

// HistoricalQuote 定义历史行情数据点
type HistoricalQuote struct {
	Timestamp time.Time
	Symbol    string
	BidPrice  decimal.Decimal
	AskPrice  decimal.Decimal
	LastPrice decimal.Decimal
}

// ClickHouseFetcher 从 ClickHouse 读取历史数据进行回测
type ClickHouseFetcher struct {
	conn driver.Conn
}

func NewClickHouseFetcher(conn driver.Conn) *ClickHouseFetcher {
	return &ClickHouseFetcher{conn: conn}
}

// FetchQuotes 分页获取历史行情数据
func (f *ClickHouseFetcher) FetchQuotes(ctx context.Context, symbol string, start, end time.Time, limit int) ([]HistoricalQuote, error) {
	query := `
		SELECT 
			symbol, 
			bid_price, 
			ask_price, 
			last_price, 
			timestamp 
		FROM quotes 
		WHERE symbol = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
		LIMIT ?
	`
	rows, err := f.conn.Query(ctx, query, symbol, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query clickhouse: %w", err)
	}
	defer rows.Close()

	var quotes []HistoricalQuote
	for rows.Next() {
		var q HistoricalQuote
		var bid, ask, last float64
		if err := rows.Scan(&q.Symbol, &bid, &ask, &last, &q.Timestamp); err != nil {
			return nil, err
		}
		q.BidPrice = decimal.NewFromFloat(bid)
		q.AskPrice = decimal.NewFromFloat(ask)
		q.LastPrice = decimal.NewFromFloat(last)
		quotes = append(quotes, q)
	}

	return quotes, nil
}
