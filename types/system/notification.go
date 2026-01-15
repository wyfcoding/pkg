package system

// NotificationMessage 系统通知消息载荷.
type NotificationMessage struct {
	Metadata       map[string]string `json:"metadata,omitempty"`
	ReadAt         *int64            `json:"read_at,omitempty"`
	NotificationID string            `json:"notification_id"`
	UserID         string            `json:"user_id"`
	Type           string            `json:"type"` // ORDER, TRADE, RISK, SYSTEM
	Title          string            `json:"title"`
	Content        string            `json:"content"`
	CreatedAt      int64             `json:"created_at"`
	IsRead         bool              `json:"is_read"`
}
