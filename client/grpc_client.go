package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type ClientConfig struct {
	Address           string
	Timeout           time.Duration
	MaxRecvMsgSize    int
	MaxSendMsgSize    int
	EnableTLS         bool
	CertFile          string
	KeyFile           string
	InsecureSkipVerify bool
	KeepaliveTime     time.Duration
	KeepaliveTimeout  time.Duration
	RetryCount        int
	RetryDelay        time.Duration
}

type ClientFactory struct {
	config    ClientConfig
	conn      *grpc.ClientConn
	keepalive keepalive.ClientParameters
}

func NewClientFactory(config ClientConfig) *ClientFactory {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRecvMsgSize == 0 {
		config.MaxRecvMsgSize = 10 * 1024 * 1024
	}
	if config.MaxSendMsgSize == 0 {
		config.MaxSendMsgSize = 10 * 1024 * 1024
	}
	if config.KeepaliveTime == 0 {
		config.KeepaliveTime = 30 * time.Second
	}
	if config.KeepaliveTimeout == 0 {
		config.KeepaliveTimeout = 10 * time.Second
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}

	return &ClientFactory{
		config: config,
		keepalive: keepalive.ClientParameters{
			Time:                config.KeepaliveTime,
			Timeout:             config.KeepaliveTimeout,
			PermitWithoutStream: true,
		},
	}
}

func (f *ClientFactory) Connect(ctx context.Context) error {
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(f.keepalive),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(f.config.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(f.config.MaxSendMsgSize),
		),
	}

	if f.config.EnableTLS {
		var creds credentials.TransportCredentials
		if f.config.CertFile != "" && f.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(f.config.CertFile, f.config.KeyFile)
			if err != nil {
				return fmt.Errorf("load certificate: %w", err)
			}
			creds = credentials.NewTLS(&tls.Config{
				Certificates:       []tls.Certificate{cert},
				InsecureSkipVerify: f.config.InsecureSkipVerify,
			})
		} else {
			creds = credentials.NewTLS(&tls.Config{
				InsecureSkipVerify: f.config.InsecureSkipVerify,
			})
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, f.config.Address, opts...)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	f.conn = conn
	return nil
}

func (f *ClientFactory) Connection() *grpc.ClientConn {
	return f.conn
}

func (f *ClientFactory) Close() error {
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}

func (f *ClientFactory) WithTimeout(ctx context.Context, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, f.config.Timeout)
	defer cancel()
	return fn(ctx)
}

func (f *ClientFactory) WithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error
	for i := 0; i < f.config.RetryCount; i++ {
		if err := fn(ctx); err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(f.config.RetryDelay):
				continue
			}
		}
		return nil
	}
	return lastErr
}
