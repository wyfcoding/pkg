package analytics

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// SecurityAuditRecord 安全审计记录
type SecurityAuditRecord struct {
	Timestamp    time.Time
	UserID       string
	Event        string // e.g., "LOGIN", "API_KEY_CREATED", "UNAUTHORIZED_ACCESS"
	Status       string // "SUCCESS", "FAILURE"
	IPAddress    string
	UserAgent    string
	RequestURL   string
	ErrorMessage string
}

// SecurityAuditor 负责安全审计日志存储
type SecurityAuditor struct {
	conn driver.Conn
}

func NewSecurityAuditor(conn driver.Conn) *SecurityAuditor {
	return &SecurityAuditor{conn: conn}
}

// Audit 记录安全审计事件
func (a *SecurityAuditor) Audit(ctx context.Context, record SecurityAuditRecord) error {
	query := `
		INSERT INTO security_audits (
			timestamp, user_id, event, status, ip_address, user_agent, request_url, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	err := a.conn.Exec(ctx, query,
		record.Timestamp,
		record.UserID,
		record.Event,
		record.Status,
		record.IPAddress,
		record.UserAgent,
		record.RequestURL,
		record.ErrorMessage,
	)
	return err
}

// SQL 迁移建议:
/*
CREATE TABLE IF NOT EXISTS security_audits (
    timestamp DateTime64(3, 'UTC'),
    user_id String,
    event String,
    status String,
    ip_address String,
    user_agent String,
    request_url String,
    error_message String
) ENGINE = MergeTree()
ORDER BY (timestamp, user_id, event);
*/
