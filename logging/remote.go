// Package logging 提供了基于标准库 slog 深度定制的生产级日志系统.
// 生成摘要:
// 1) 增加异步远程日志写入能力，支持批量与失败隔离。
// 假设:
// 1) 远端接受 application/x-ndjson 格式的日志数据。
package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// RemoteConfig 定义远程日志写入配置。
type RemoteConfig struct {
	Enabled       bool
	Endpoint      string
	AuthToken     string
	Timeout       time.Duration
	BatchSize     int
	BufferSize    int
	FlushInterval time.Duration
	DropOnFull    bool
}

type remoteWriter struct {
	endpoint      string
	authToken     string
	timeout       time.Duration
	batchSize     int
	flushInterval time.Duration
	dropOnFull    bool

	queue   chan []byte
	stopCh  chan struct{}
	wg      sync.WaitGroup
	dropped atomic.Int64
	closed  atomic.Bool
	client  *http.Client
	logger  *slog.Logger
}

func newRemoteWriter(cfg RemoteConfig) (*remoteWriter, func() error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 2 * time.Second
	}

	w := &remoteWriter{
		endpoint:      cfg.Endpoint,
		authToken:     cfg.AuthToken,
		timeout:       timeout,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		dropOnFull:    cfg.DropOnFull,
		queue:         make(chan []byte, bufferSize),
		stopCh:        make(chan struct{}),
		client: &http.Client{
			Timeout: timeout,
		},
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}

	w.wg.Add(1)
	go w.loop()

	return w, w.close
}

func (w *remoteWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.closed.Load() {
		return len(p), nil
	}

	cp := make([]byte, len(p))
	copy(cp, p)

	if w.dropOnFull {
		select {
		case w.queue <- cp:
		default:
			w.dropped.Add(1)
		}
		return len(p), nil
	}

	w.queue <- cp
	return len(p), nil
}

func (w *remoteWriter) loop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	var batch [][]byte

	flush := func() {
		if len(batch) == 0 {
			return
		}

		payload := bytes.Buffer{}
		for _, item := range batch {
			payload.Write(bytes.TrimRight(item, "\n"))
			payload.WriteByte('\n')
		}

		ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.endpoint, bytes.NewReader(payload.Bytes()))
		if err != nil {
			w.logger.Warn("remote log request build failed", "error", err)
			batch = batch[:0]
			return
		}

		req.Header.Set("Content-Type", "application/x-ndjson")
		if w.authToken != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", w.authToken))
		}

		resp, err := w.client.Do(req)
		if err != nil {
			w.logger.Warn("remote log request failed", "error", err)
			batch = batch[:0]
			return
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			w.logger.Warn("remote log request returned non-2xx", "status", resp.Status)
		}

		batch = batch[:0]
	}

	for {
		select {
		case item := <-w.queue:
			batch = append(batch, item)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.stopCh:
			flush()
			dropped := w.dropped.Load()
			if dropped > 0 {
				w.logger.Warn("remote log dropped messages", "count", dropped)
			}
			return
		}
	}
}

func (w *remoteWriter) close() error {
	w.closed.Store(true)
	close(w.stopCh)
	w.wg.Wait()
	return nil
}
