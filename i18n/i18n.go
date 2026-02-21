package i18n

import (
	"encoding/json"
	"fmt"
	"sync"
)

// 生成摘要：实现轻量级高性能国际化库。
// 支持多语言包加载与并发读取。

type Bundle struct {
	mu       sync.RWMutex
	messages map[string]map[string]string // lang -> key -> message
}

func NewBundle() *Bundle {
	return &Bundle{
		messages: make(map[string]map[string]string),
	}
}

// LoadLanguage 加载语言 JSON 数据。
func (b *Bundle) LoadLanguage(lang string, data []byte) error {
	var msgs map[string]string
	if err := json.Unmarshal(data, &msgs); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages[lang] = msgs
	return nil
}

// Get 获取指定语言的翻译。
func (b *Bundle) Get(lang, key string, args ...any) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	langMsgs, ok := b.messages[lang]
	if !ok {
		return key
	}

	msg, ok := langMsgs[key]
	if !ok {
		return key
	}

	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}
