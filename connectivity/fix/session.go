package fix

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrSeqNumTooLow 序列号过低.
	ErrSeqNumTooLow = errors.New("seq num too low")
	// ErrSeqNumGap 序列号存在间隔.
	ErrSeqNumGap = errors.New("seq num gap detected")
)

// Session 维护一个 FIX 连接的状态
type Session struct {
	LastHeartbeat time.Time
	mu            sync.RWMutex
	ID            string
	SenderCompID  string
	TargetCompID  string
	InSeqNum      int64
	OutSeqNum     int64
}

func NewSession(id, sender, target string) *Session {
	return &Session{
		ID:           id,
		SenderCompID: sender,
		TargetCompID: target,
		InSeqNum:     1,
		OutSeqNum:    1,
	}
}

// NextOutSeq 获取下一个发出的序列号
func (s *Session) NextOutSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	seq := s.OutSeqNum
	s.OutSeqNum++
	return seq
}

// ValidateInSeq 验证收到的序列号
func (s *Session) ValidateInSeq(seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq < s.InSeqNum {
		return fmt.Errorf("%w: got %d, want %d", ErrSeqNumTooLow, seq, s.InSeqNum)
	}
	if seq > s.InSeqNum {
		// 实际应触发 ResendRequest
		return fmt.Errorf("%w: got %d, want %d", ErrSeqNumGap, seq, s.InSeqNum)
	}
	s.InSeqNum++
	s.LastHeartbeat = time.Now()
	return nil
}

// SessionManager 集中管理所有 FIX 会话
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (m *SessionManager) AddSession(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
}

func (m *SessionManager) GetSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
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
