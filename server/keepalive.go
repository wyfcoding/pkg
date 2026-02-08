package server

import (
	"github.com/wyfcoding/pkg/config"

	"google.golang.org/grpc/keepalive"
)

// toServerParams 构造 gRPC keepalive.ServerParameters。
func toServerParams(cfg config.GRPCKeepaliveConfig) keepalive.ServerParameters {
	return keepalive.ServerParameters{
		MaxConnectionIdle:     cfg.MaxConnectionIdle,
		MaxConnectionAge:      cfg.MaxConnectionAge,
		MaxConnectionAgeGrace: cfg.MaxConnectionAgeGrace,
		Time:                  cfg.Time,
		Timeout:               cfg.Timeout,
	}
}

// toEnforcement 构造 gRPC keepalive.EnforcementPolicy。
func toEnforcement(cfg config.GRPCKeepaliveConfig) keepalive.EnforcementPolicy {
	return keepalive.EnforcementPolicy{
		MinTime:             cfg.MinTime,
		PermitWithoutStream: cfg.PermitWithoutStream,
	}
}
