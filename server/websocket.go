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
	conn    *websocket.Conn
	send    chan []byte
	manager *WSManager
	topics  map[string]struct{} // 该客户端订阅的主.
	mu      sync.Mutex
}

// WSManager 管理所有活跃的 WebSocket 连.
// 优化：字段按大小排序以减少内存对齐填充。
type WSManager struct {
	clients    map[*Client]bool
	broadcast  chan BroadcastMessage
	register   chan *Client
	unregister chan *Client
	logger     *slog.Logger
	mu         sync.RWMutex
}

type BroadcastMessage struct {
	Topic   string
	Payload []byte
}

func NewWSManager(logger *slog.Logger) *WSManager {
	return &WSManager{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan BroadcastMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger.With("module", "websocket_manager"),
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
			m.mu.Unlock()
			m.logger.Debug("client registered", "addr", client.conn.RemoteAddr())
		case client := <-m.unregister:
			m.mu.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				close(client.send)
			}
			m.mu.Unlock()
			m.logger.Debug("client unregistered", "addr", client.conn.RemoteAddr())
		case message := <-m.broadcast:
			m.mu.RLock()
			for client := range m.clients {
				client.mu.Lock()
				// 检查客户端是否订阅了该主.
				if _, ok := client.topics[message.Topic]; ok {
					select {
					case client.send <- message.Payload:
					default:
						// 如果缓冲区满了，主动断开客户端以防止阻塞整个广.
						m.logger.Warn("client buffer full, dropping", "addr", client.conn.RemoteAddr())
						go func(c *Client) { m.unregister <- c }(client)
					}
				}
				client.mu.Unlock()
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

// ServeHTTP 处理 WebSocket 升级请.
func (m *WSManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		conn:    conn,
		send:    make(chan []byte, 256),
		manager: m,
		topics:  make(map[string]struct{}),
	}

	m.register <- client

	// 启动写协.
	go client.writePump()
	// 启动读协程 (处理订阅/心跳.
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, message, err := c.conn.ReadMessage()
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
				c.topics[cmd.Topic] = struct{}{}
			case "unsubscribe":
				delete(c.topics, cmd.Topic)
			}
			c.mu.Unlock()
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					// 仅记录 debug，因为连接可能已经关.
					c.manager.logger.Debug("failed to write close message", "error", err)
				}
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
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
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
