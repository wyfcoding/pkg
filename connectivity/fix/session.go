package fix

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSeqNumTooLow     = errors.New("seq num too low")
	ErrSeqNumGap        = errors.New("seq num gap detected")
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionClosed    = errors.New("session closed")
	ErrInvalidMessage   = errors.New("invalid message")
	ErrHeartbeatTimeout = errors.New("heartbeat timeout")
	ErrNotLoggedIn      = errors.New("not logged in")
)

type SessionState int

const (
	SessionStateDisconnected SessionState = iota
	SessionStateConnecting
	SessionStateLogonSent
	SessionStateLogonReceived
	SessionStateActive
	SessionStateLogoutSent
	SessionStateLogoutReceived
	SessionStateResending
)

func (s SessionState) String() string {
	switch s {
	case SessionStateDisconnected:
		return "DISCONNECTED"
	case SessionStateConnecting:
		return "CONNECTING"
	case SessionStateLogonSent:
		return "LOGON_SENT"
	case SessionStateLogonReceived:
		return "LOGON_RECEIVED"
	case SessionStateActive:
		return "ACTIVE"
	case SessionStateLogoutSent:
		return "LOGOUT_SENT"
	case SessionStateLogoutReceived:
		return "LOGOUT_RECEIVED"
	case SessionStateResending:
		return "RESENDING"
	default:
		return "UNKNOWN"
	}
}

type SessionConfig struct {
	SenderCompID       string
	TargetCompID       string
	HeartBtInt         int
	EncryptMethod      int
	ResetSeqNum        bool
	StartTime          string
	EndTime            string
	ReconnectInterval  time.Duration
	MaxReconnectTries  int
	HeartbeatTimeout   time.Duration
	HeartbeatInterval  time.Duration
	MaxMessageSize     int
	EnableLastMsgSeqNum bool
}

type Session struct {
	ID             string
	SenderCompID   string
	TargetCompID   string
	State          SessionState
	InSeqNum       int64
	OutSeqNum      int64
	LastHeartbeat  time.Time
	LastReceived   time.Time
	LastSent       time.Time
	HeartBtInt     int
	CreateTime     time.Time
	DisconnectTime time.Time
	RemoteIP       string
	RemotePort     int
	Config         SessionConfig
	mu             sync.RWMutex
	msgQueue       chan *Message
	stopChan       chan struct{}
	onMessage      func(*Session, *Message)
	onStateChange  func(*Session, SessionState, SessionState)
	onError        func(*Session, error)
}

func NewSession(id, sender, target string, config SessionConfig) *Session {
	return &Session{
		ID:            id,
		SenderCompID:  sender,
		TargetCompID:  target,
		State:         SessionStateDisconnected,
		InSeqNum:      1,
		OutSeqNum:     1,
		HeartBtInt:    config.HeartBtInt,
		CreateTime:    time.Now(),
		Config:        config,
		msgQueue:      make(chan *Message, 10000),
		stopChan:      make(chan struct{}),
	}
}

func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != SessionStateDisconnected {
		return errors.New("session already started")
	}

	s.State = SessionStateConnecting
	s.notifyStateChange(SessionStateDisconnected, SessionStateConnecting)

	go s.heartbeatLoop()
	go s.messageLoop()

	return nil
}

func (s *Session) Stop() {
	s.mu.Lock()
	if s.State == SessionStateDisconnected {
		s.mu.Unlock()
		return
	}
	s.State = SessionStateDisconnected
	s.DisconnectTime = time.Now()
	close(s.stopChan)
	s.mu.Unlock()
}

func (s *Session) NextOutSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	seq := s.OutSeqNum
	s.OutSeqNum++
	s.LastSent = time.Now()
	return seq
}

func (s *Session) ValidateInSeq(seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if seq < s.InSeqNum {
		return fmt.Errorf("%w: got %d, want %d", ErrSeqNumTooLow, seq, s.InSeqNum)
	}
	if seq > s.InSeqNum {
		return fmt.Errorf("%w: got %d, want %d", ErrSeqNumGap, seq, s.InSeqNum)
	}
	s.InSeqNum++
	s.LastReceived = time.Now()
	s.LastHeartbeat = time.Now()
	return nil
}

func (s *Session) SetState(state SessionState) {
	s.mu.Lock()
	oldState := s.State
	s.State = state
	s.mu.Unlock()
	s.notifyStateChange(oldState, state)
}

func (s *Session) GetState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == SessionStateActive
}

func (s *Session) UpdateHeartbeat() {
	s.mu.Lock()
	s.LastHeartbeat = time.Now()
	s.mu.Unlock()
}

func (s *Session) CheckHeartbeatTimeout() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	timeout := s.Config.HeartbeatTimeout
	if timeout == 0 {
		timeout = time.Duration(s.HeartBtInt*2) * time.Second
	}
	return time.Since(s.LastHeartbeat) > timeout
}

func (s *Session) SendMessage(msg *Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.State != SessionStateActive {
		return ErrSessionClosed
	}

	select {
	case s.msgQueue <- msg:
		return nil
	default:
		return errors.New("message queue full")
	}
}

func (s *Session) SetMessageHandler(handler func(*Session, *Message)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMessage = handler
}

func (s *Session) SetStateChangeHandler(handler func(*Session, SessionState, SessionState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStateChange = handler
}

func (s *Session) SetErrorHandler(handler func(*Session, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = handler
}

func (s *Session) heartbeatLoop() {
	ticker := time.NewTicker(time.Duration(s.HeartBtInt) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if s.CheckHeartbeatTimeout() {
				s.notifyError(ErrHeartbeatTimeout)
				s.SetState(SessionStateDisconnected)
				return
			}
		}
	}
}

func (s *Session) messageLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		case msg := <-s.msgQueue:
			if s.onMessage != nil {
				s.onMessage(s, msg)
			}
		}
	}
}

func (s *Session) notifyStateChange(oldState, newState SessionState) {
	if s.onStateChange != nil {
		s.onStateChange(s, oldState, newState)
	}
}

func (s *Session) notifyError(err error) {
	if s.onError != nil {
		s.onError(s, err)
	}
}

func (s *Session) ResetSeqNum() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InSeqNum = 1
	s.OutSeqNum = 1
}

func (s *Session) GetStats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SessionStats{
		ID:              s.ID,
		SenderCompID:    s.SenderCompID,
		TargetCompID:    s.TargetCompID,
		State:           s.State.String(),
		InSeqNum:        s.InSeqNum,
		OutSeqNum:       s.OutSeqNum,
		LastHeartbeat:   s.LastHeartbeat,
		LastReceived:    s.LastReceived,
		LastSent:        s.LastSent,
		CreateTime:      s.CreateTime,
		DisconnectTime:  s.DisconnectTime,
		HeartBtInt:      s.HeartBtInt,
	}
}

type SessionStats struct {
	ID             string    `json:"id"`
	SenderCompID   string    `json:"sender_comp_id"`
	TargetCompID   string    `json:"target_comp_id"`
	State          string    `json:"state"`
	InSeqNum       int64     `json:"in_seq_num"`
	OutSeqNum      int64     `json:"out_seq_num"`
	LastHeartbeat  time.Time `json:"last_heartbeat"`
	LastReceived   time.Time `json:"last_received"`
	LastSent       time.Time `json:"last_sent"`
	CreateTime     time.Time `json:"create_time"`
	DisconnectTime time.Time `json:"disconnect_time"`
	HeartBtInt     int       `json:"heart_bt_int"`
}

type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (m *SessionManager) AddSession(s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[s.ID]; exists {
		return errors.New("session already exists")
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *SessionManager) GetSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *SessionManager) RemoveSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, exists := m.sessions[id]; exists {
		s.Stop()
		delete(m.sessions, id)
	}
}

func (m *SessionManager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return list
}

func (m *SessionManager) GetActiveSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Session, 0)
	for _, s := range m.sessions {
		if s.IsActive() {
			list = append(list, s)
		}
	}
	return list
}

func (m *SessionManager) GetSessionStats() []SessionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make([]SessionStats, 0, len(m.sessions))
	for _, s := range m.sessions {
		stats = append(stats, s.GetStats())
	}
	return stats
}

func (m *SessionManager) Broadcast(msg *Message) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	for _, s := range m.sessions {
		if s.IsActive() {
			if err := s.SendMessage(msg); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

func (m *SessionManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.Stop()
	}
	m.sessions = make(map[string]*Session)
}
