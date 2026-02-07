package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		// 解析 Origin 头部.
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}

		// 生产环境真实逻辑：同源检查.
		// 校验 Origin 的 Host 是否与请求的 Host 匹配.
		// 注意：r.Host 可能包含端口，需要处理.
		requestHost := r.Host
		originHost := u.Host

		if h, _, err := net.SplitHostPort(requestHost); err == nil {
			requestHost = h
		}
		if h, _, err := net.SplitHostPort(originHost); err == nil {
			originHost = h
		}

		// 允许同源请求.
		if strings.EqualFold(requestHost, originHost) {
			return true
		}

		// 特殊处理：如果是本地开发环境，允许 localhost 访问.
		return originHost == "localhost" || originHost == "127.0.0.1"
	},
}

// Client 表示一个 WebSocket 客户端连.
type Client struct {
	Conn    *websocket.Conn
	Send    chan []byte
	Manager *WSManager
	Topics  map[string]struct{} // 该客户端订阅的主.
	UserID  string              // 用户ID，用于定向推送
	mu      sync.Mutex
}

// WSManager 管理所有活跃的 WebSocket 连.
// 优化：字段按大小排序以减少内存对齐填充。
type WSManager struct {
	clients     map[*Client]bool
	userClients map[string]map[*Client]bool // 按UserID分组的客户端
	broadcast   chan BroadcastMessage
	userMessage chan UserMessage // 用户定向消息通道
	register    chan *Client
	unregister  chan *Client
	logger      *slog.Logger
	mu          sync.RWMutex
}

// UserMessage 表示发送给指定用户的消息
type UserMessage struct {
	UserID  string
	Payload []byte
}

type BroadcastMessage struct {
	Topic   string
	Payload []byte
}

func NewWSManager(logger *slog.Logger) *WSManager {
	return &WSManager{
		clients:     make(map[*Client]bool),
		userClients: make(map[string]map[*Client]bool),
		broadcast:   make(chan BroadcastMessage),
		userMessage: make(chan UserMessage, 256),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		logger:      logger.With("module", "websocket_manager"),
	}
}

func (m *WSManager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-m.register:
			m.mu.Lock()
			m.clients[client] = true
			// 按UserID索引
			if client.UserID != "" {
				if m.userClients[client.UserID] == nil {
					m.userClients[client.UserID] = make(map[*Client]bool)
				}
				m.userClients[client.UserID][client] = true
			}
			m.mu.Unlock()
			m.logger.Debug("client registered", "addr", client.Conn.RemoteAddr(), "user_id", client.UserID)
		case client := <-m.unregister:
			m.mu.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				// 从 UserID 索引中移除
				if client.UserID != "" {
					if userClients, ok := m.userClients[client.UserID]; ok {
						delete(userClients, client)
						if len(userClients) == 0 {
							delete(m.userClients, client.UserID)
						}
					}
				}
				close(client.Send)
			}
			m.mu.Unlock()
			m.logger.Debug("client unregistered", "addr", client.Conn.RemoteAddr())
		case message := <-m.broadcast:
			m.mu.RLock()
			for client := range m.clients {
				client.mu.Lock()
				// 检查客户端是否订阅了该主.
				if _, ok := client.Topics[message.Topic]; ok {
					select {
					case client.Send <- message.Payload:
					default:
						// 如果缓冲区满了，主动断开客户端以防止阻塞整个广.
						m.logger.Warn("client buffer full, dropping", "addr", client.Conn.RemoteAddr())
						go func(c *Client) { m.unregister <- c }(client)
					}
				}
				client.mu.Unlock()
			}
			m.mu.RUnlock()
		case message := <-m.userMessage:
			// 用户定向消息
			m.mu.RLock()
			if clients, ok := m.userClients[message.UserID]; ok {
				for client := range clients {
					select {
					case client.Send <- message.Payload:
					default:
						m.logger.Warn("user client buffer full, dropping", "user_id", message.UserID)
						go func(c *Client) { m.unregister <- c }(client)
					}
				}
			}
			m.mu.RUnlock()
		}
	}
}

// Broadcast 对外发布的广播接口 (会自动序列化 payload.
func (m *WSManager) Broadcast(topic string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Error("failed to marshal broadcast data", "error", err)
		return
	}
	m.BroadcastRaw(topic, data)
}

// BroadcastRaw 广播原始字节数据 (高性能入口，不进行重复序列化.
func (m *WSManager) BroadcastRaw(topic string, payload []byte) {
	m.broadcast <- BroadcastMessage{Topic: topic, Payload: payload}
}

// SendToUser 向指定用户发送消息 (会自动序列化 payload)
func (m *WSManager) SendToUser(userID string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Error("failed to marshal user message data", "error", err)
		return
	}
	m.SendToUserRaw(userID, data)
}

// SendToUserRaw 向指定用户发送原始字节数据
func (m *WSManager) SendToUserRaw(userID string, payload []byte) {
	m.userMessage <- UserMessage{UserID: userID, Payload: payload}
}

// IsUserOnline 检查用户是否在线
func (m *WSManager) IsUserOnline(userID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	clients, ok := m.userClients[userID]
	return ok && len(clients) > 0
}

// GetOnlineUserCount 获取在线用户数
func (m *WSManager) GetOnlineUserCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.userClients)
}

// Register 注册客户端
func (m *WSManager) Register(client *Client) {
	m.register <- client
}

// Unregister 注销客户端
func (m *WSManager) Unregister(client *Client) {
	m.unregister <- client
}

// ServeHTTP 处理 WebSocket 升级请.
func (m *WSManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Manager: m,
		Topics:  make(map[string]struct{}),
	}

	m.register <- client

	// 启动写协.
	go client.WritePump()
	// 启动读协程 (处理订阅/心跳.
	go client.ReadPump()
}

func (c *Client) ReadPump() {
	defer func() {
		c.Manager.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	if err := c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return
	}
	c.Conn.SetPongHandler(func(string) error {
		return c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		// 解析客户端命令 (如: {"op": "subscribe", "topic": "BTC/USDT"}.
		var cmd struct {
			Op    string `json:"op"`
			Topic string `json:"topic"`
		}
		if err := json.Unmarshal(message, &cmd); err == nil {
			c.mu.Lock()
			switch cmd.Op {
			case "subscribe":
				c.Topics[cmd.Topic] = struct{}{}
			case "unsubscribe":
				delete(c.Topics, cmd.Topic)
			}
			c.mu.Unlock()
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if !ok {
				if err := c.Conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					// 仅记录 debug，因为连接可能已经关.
					c.Manager.logger.Debug("failed to write close message", "error", err)
				}
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
