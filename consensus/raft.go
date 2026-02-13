// 变更说明：新增分布式一致性组件，实现简化的Raft共识算法
package consensus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// State 节点状态
type State int32

const (
	StateFollower State = iota
	StateCandidate
	StateLeader
)

// Command 日志命令
type Command struct {
	Index     uint64
	Term      uint64
	Operation string
	Data      []byte
	Timestamp int64
}

// LogEntry 日志条目
type LogEntry struct {
	Term    uint64
	Index   uint64
	Command Command
}

// VoteRequest 投票请求
type VoteRequest struct {
	Term         uint64
	CandidateID  string
	LastLogIndex uint64
	LastLogTerm  uint64
}

// VoteResponse 投票响应
type VoteResponse struct {
	Term        uint64
	VoteGranted bool
	VoterID     string
}

// AppendEntriesRequest 追加日志请求
type AppendEntriesRequest struct {
	Term         uint64
	LeaderID     string
	PrevLogIndex uint64
	PrevLogTerm  uint64
	Entries      []LogEntry
	LeaderCommit uint64
}

// AppendEntriesResponse 追加日志响应
type AppendEntriesResponse struct {
	Term    uint64
	Success bool
	NextIndex uint64
}

// Node Raft节点
type Node struct {
	nodeID        string
	peers         map[string]*Peer
	state         int32
	currentTerm   uint64
	votedFor      string
	votes         map[string]bool
	log           []LogEntry
	commitIndex   uint64
	lastApplied   uint64
	nextIndex     map[string]uint64
	matchIndex    map[string]uint64
	leaderID      string
	electionTimer *time.Timer
	heartbeatTick *time.Ticker
	lastHeartbeat time.Time
	electionTimeout time.Duration
	heartbeatInterval time.Duration
	applyCh       chan Command
	voteRequestCh       chan *VoteRequest
	voteResponseCh      chan *VoteResponse
	appendEntriesCh     chan *AppendEntriesRequest
	appendEntriesResponseCh chan *AppendEntriesResponse
	proposeCh           chan Command
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// Peer 对等节点
type Peer struct {
	NodeID   string
	Address  string
	NextIndex uint64
	MatchIndex uint64
}

// StateMachine 状态机接口
type StateMachine interface {
	Apply(command Command) error
	Snapshot() ([]byte, error)
	Restore(snapshot []byte) error
}

// Config 配置
type Config struct {
	NodeID           string
	Peers            []string
	ElectionTimeout  time.Duration
	HeartbeatInterval time.Duration
}

// NewNode 创建Raft节点
func NewNode(config Config) *Node {
	ctx, cancel := context.WithCancel(context.Background())
	
	peers := make(map[string]*Peer)
	for _, peerID := range config.Peers {
		if peerID != config.NodeID {
			peers[peerID] = &Peer{
				NodeID: peerID,
			}
		}
	}
	
	electionTimeout := config.ElectionTimeout
	if electionTimeout == 0 {
		electionTimeout = 300 * time.Millisecond
	}
	
	heartbeatInterval := config.HeartbeatInterval
	if heartbeatInterval == 0 {
		heartbeatInterval = 100 * time.Millisecond
	}
	
	return &Node{
		nodeID:           config.NodeID,
		peers:            peers,
		state:            int32(StateFollower),
		currentTerm:      0,
		votedFor:         "",
		votes:            make(map[string]bool),
		log:              make([]LogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		nextIndex:        make(map[string]uint64),
		matchIndex:       make(map[string]uint64),
		electionTimeout:  electionTimeout,
		heartbeatInterval: heartbeatInterval,
		applyCh:          make(chan Command, 1000),
		voteRequestCh:    make(chan *VoteRequest, 100),
		voteResponseCh:   make(chan *VoteResponse, 100),
		appendEntriesCh:  make(chan *AppendEntriesRequest, 100),
		appendEntriesResponseCh: make(chan *AppendEntriesResponse, 100),
		proposeCh:        make(chan Command, 1000),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start 启动节点
func (n *Node) Start() {
	n.resetElectionTimer()
	n.wg.Add(1)
	go n.run()
}

// Stop 停止节点
func (n *Node) Stop() {
	n.cancel()
	n.wg.Wait()
	close(n.applyCh)
}

// run 主循环
func (n *Node) run() {
	defer n.wg.Done()
	
	for {
		select {
		case <-n.ctx.Done():
			return
		default:
			state := State(atomic.LoadInt32(&n.state))
			
			switch state {
			case StateFollower:
				n.runFollower()
			case StateCandidate:
				n.runCandidate()
			case StateLeader:
				n.runLeader()
			}
		}
	}
}

// runFollower 跟随者状态
func (n *Node) runFollower() {
	select {
	case <-n.ctx.Done():
		return
	case <-n.electionTimer.C:
		n.becomeCandidate()
	case req := <-n.voteRequestCh:
		n.handleVoteRequest(req)
	case req := <-n.appendEntriesCh:
		n.handleAppendEntries(req)
	}
}

// runCandidate 候选者状态
func (n *Node) runCandidate() {
	n.startElection()
	
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.electionTimer.C:
			n.becomeCandidate()
		case req := <-n.voteRequestCh:
			n.handleVoteRequest(req)
		case req := <-n.appendEntriesCh:
			if req.Term >= n.currentTerm {
				n.becomeFollower(req.Term, req.LeaderID)
			}
			n.handleAppendEntries(req)
			return
		case vote := <-n.voteResponseCh:
			if vote.VoteGranted {
				n.votes[vote.VoterID] = true
				if n.hasMajority() {
					n.becomeLeader()
					return
				}
			}
		}
	}
}

// runLeader 领导者状态
func (n *Node) runLeader() {
	n.heartbeatTick = time.NewTicker(n.heartbeatInterval)
	defer n.heartbeatTick.Stop()
	
	n.sendHeartbeats()
	
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.heartbeatTick.C:
			n.sendHeartbeats()
		case req := <-n.voteRequestCh:
			if req.Term > n.currentTerm {
				n.becomeFollower(req.Term, "")
				n.handleVoteRequest(req)
				return
			}
		case req := <-n.appendEntriesCh:
			if req.Term > n.currentTerm {
				n.becomeFollower(req.Term, req.LeaderID)
				n.handleAppendEntries(req)
				return
			}
		case resp := <-n.appendEntriesResponseCh:
			n.handleAppendEntriesResponse(resp)
		case cmd := <-n.proposeCh:
			n.propose(cmd)
		}
	}
}

// becomeFollower 成为跟随者
func (n *Node) becomeFollower(term uint64, leaderID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	atomic.StoreInt32(&n.state, int32(StateFollower))
	n.currentTerm = term
	n.leaderID = leaderID
	n.votedFor = ""
	n.resetElectionTimer()
}

// becomeCandidate 成为候选者
func (n *Node) becomeCandidate() {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	atomic.StoreInt32(&n.state, int32(StateCandidate))
	n.currentTerm++
	n.votedFor = n.nodeID
	n.votes = make(map[string]bool)
	n.votes[n.nodeID] = true
	n.resetElectionTimer()
}

// becomeLeader 成为领导者
func (n *Node) becomeLeader() {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	atomic.StoreInt32(&n.state, int32(StateLeader))
	n.leaderID = n.nodeID
	
	lastLogIndex := n.getLastLogIndex()
	for peerID := range n.peers {
		n.nextIndex[peerID] = lastLogIndex + 1
		n.matchIndex[peerID] = 0
	}
}

// startElection 开始选举
func (n *Node) startElection() {
	n.mu.RLock()
	lastLogIndex := n.getLastLogIndex()
	lastLogTerm := n.getLastLogTerm()
	n.mu.RUnlock()
	
	req := &VoteRequest{
		Term:         n.currentTerm,
		CandidateID:  n.nodeID,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}
	
	for peerID := range n.peers {
		go n.sendVoteRequest(peerID, req)
	}
}

// sendHeartbeats 发送心跳
func (n *Node) sendHeartbeats() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	for peerID := range n.peers {
		req := n.buildAppendEntriesRequest(peerID)
		go n.sendAppendEntries(peerID, req)
	}
}

// buildAppendEntriesRequest 构建追加日志请求
func (n *Node) buildAppendEntriesRequest(peerID string) *AppendEntriesRequest {
	nextIdx := n.nextIndex[peerID]
	prevLogIndex := nextIdx - 1
	prevLogTerm := uint64(0)
	
	if prevLogIndex > 0 {
		prevLogTerm = n.log[prevLogIndex-1].Term
	}
	
	var entries []LogEntry
	if nextIdx <= uint64(len(n.log)) {
		entries = n.log[nextIdx-1:]
	}
	
	return &AppendEntriesRequest{
		Term:         n.currentTerm,
		LeaderID:     n.nodeID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: n.commitIndex,
	}
}

// handleVoteRequest 处理投票请求
func (n *Node) handleVoteRequest(req *VoteRequest) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	resp := &VoteResponse{
		Term: n.currentTerm,
	}
	
	if req.Term < n.currentTerm {
		resp.VoteGranted = false
		n.sendVoteResponse(req.CandidateID, resp)
		return
	}
	
	if req.Term > n.currentTerm {
		n.currentTerm = req.Term
		n.votedFor = ""
		n.becomeFollower(req.Term, "")
	}
	
	if n.votedFor == "" || n.votedFor == req.CandidateID {
		if n.isLogUpToDate(req.LastLogTerm, req.LastLogIndex) {
			n.votedFor = req.CandidateID
			resp.VoteGranted = true
			n.resetElectionTimer()
		}
	}
	
	n.sendVoteResponse(req.CandidateID, resp)
}

// handleAppendEntries 处理追加日志请求
func (n *Node) handleAppendEntries(req *AppendEntriesRequest) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	resp := &AppendEntriesResponse{
		Term: n.currentTerm,
	}
	
	if req.Term < n.currentTerm {
		resp.Success = false
		n.sendAppendEntriesResponse(req.LeaderID, resp)
		return
	}
	
	if req.Term > n.currentTerm {
		n.currentTerm = req.Term
		n.votedFor = ""
	}
	
	n.leaderID = req.LeaderID
	n.resetElectionTimer()
	
	if req.PrevLogIndex > 0 {
		if uint64(len(n.log)) < req.PrevLogIndex {
			resp.Success = false
			resp.NextIndex = uint64(len(n.log)) + 1
			n.sendAppendEntriesResponse(req.LeaderID, resp)
			return
		}
		
		if n.log[req.PrevLogIndex-1].Term != req.PrevLogTerm {
			resp.Success = false
			resp.NextIndex = req.PrevLogIndex
			n.sendAppendEntriesResponse(req.LeaderID, resp)
			return
		}
	}
	
	for _, entry := range req.Entries {
		if entry.Index <= uint64(len(n.log)) {
			if n.log[entry.Index-1].Term != entry.Term {
				n.log = n.log[:entry.Index-1]
				n.log = append(n.log, entry)
			}
		} else {
			n.log = append(n.log, entry)
		}
	}
	
	if req.LeaderCommit > n.commitIndex {
		lastNewIndex := n.getLastLogIndex()
		if req.LeaderCommit < lastNewIndex {
			n.commitIndex = req.LeaderCommit
		} else {
			n.commitIndex = lastNewIndex
		}
		n.applyCommitted()
	}
	
	resp.Success = true
	resp.NextIndex = n.getLastLogIndex() + 1
	n.sendAppendEntriesResponse(req.LeaderID, resp)
}

// propose 提交提案
func (n *Node) propose(cmd Command) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if State(atomic.LoadInt32(&n.state)) != StateLeader {
		return
	}
	
	entry := LogEntry{
		Term:    n.currentTerm,
		Index:   uint64(len(n.log) + 1),
		Command: cmd,
	}
	n.log = append(n.log, entry)
	
	n.matchIndex[n.nodeID] = entry.Index
	n.updateCommitIndex()
}

// updateCommitIndex 更新提交索引
func (n *Node) updateCommitIndex() {
	for idx := n.commitIndex + 1; idx <= uint64(len(n.log)); idx++ {
		if n.log[idx-1].Term == n.currentTerm {
			count := 1
			for _, matchIdx := range n.matchIndex {
				if matchIdx >= idx {
					count++
				}
			}
			if count > len(n.peers)/2 {
				n.commitIndex = idx
			}
		}
	}
	n.applyCommitted()
}

// applyCommitted 应用已提交的日志
func (n *Node) applyCommitted() {
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		entry := n.log[n.lastApplied-1]
		select {
		case n.applyCh <- entry.Command:
		default:
		}
	}
}

// 辅助方法
func (n *Node) getLastLogIndex() uint64 {
	if len(n.log) == 0 {
		return 0
	}
	return uint64(len(n.log))
}

func (n *Node) getLastLogTerm() uint64 {
	if len(n.log) == 0 {
		return 0
	}
	return n.log[len(n.log)-1].Term
}

func (n *Node) isLogUpToDate(term, index uint64) bool {
	lastTerm := n.getLastLogTerm()
	lastIndex := n.getLastLogIndex()
	
	if term != lastTerm {
		return term > lastTerm
	}
	return index >= lastIndex
}

func (n *Node) hasMajority() bool {
	return len(n.votes) > (len(n.peers)+1)/2
}

func (n *Node) resetElectionTimer() {
	if n.electionTimer != nil {
		n.electionTimer.Stop()
	}
	timeout := n.electionTimeout + time.Duration(time.Now().UnixNano()%int64(n.electionTimeout))
	n.electionTimer = time.NewTimer(timeout)
}

// 网络通信方法（需要实际实现）
func (n *Node) sendVoteRequest(peerID string, req *VoteRequest)    {}
func (n *Node) sendVoteResponse(peerID string, resp *VoteResponse) {}
func (n *Node) sendAppendEntries(peerID string, req *AppendEntriesRequest) {}
func (n *Node) sendAppendEntriesResponse(peerID string, resp *AppendEntriesResponse) {}
func (n *Node) handleAppendEntriesResponse(resp *AppendEntriesResponse) {}

// 错误定义
var (
	ErrNotLeader      = errors.New("not the leader")
	ErrTimeout        = errors.New("operation timeout")
	ErrTermMismatch   = errors.New("term mismatch")
	ErrLogMismatch    = errors.New("log mismatch")
)
