package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

const defaultKafkaHealthTimeout = 2 * time.Second

// KafkaChecker 返回 Kafka 依赖健康检查函数。
func KafkaChecker(brokers []string, timeout time.Duration) Checker {
	return KafkaCheckerWithDialer(brokers, timeout, nil)
}

// KafkaCheckerWithDialer 返回可自定义 Dialer 的 Kafka 健康检查函数。
func KafkaCheckerWithDialer(brokers []string, timeout time.Duration, dialer *kafkago.Dialer) Checker {
	return func() error {
		if len(brokers) == 0 {
			return errors.New("kafka brokers is empty")
		}
		if timeout <= 0 {
			timeout = defaultKafkaHealthTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if dialer == nil {
			dialer = &kafkago.Dialer{Timeout: timeout}
		}

		conn, err := dialer.DialContext(ctx, "tcp", brokers[0])
		if err != nil {
			return fmt.Errorf("kafka dial failed: %w", err)
		}
		defer conn.Close()

		if _, err := conn.Brokers(); err != nil {
			return fmt.Errorf("kafka brokers fetch failed: %w", err)
		}

		return nil
	}
}
