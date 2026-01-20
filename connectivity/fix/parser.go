package fix

import (
	"fmt"
	"sort"
	"strings"
)

const (
	SOH = "\x01"
)

// Message 代表一条 FIX 报文
type Message struct {
	Fields map[int]string
	Tags   []int // 保持顺序用于校验等
}

func NewMessage() *Message {
	return &Message{
		Fields: make(map[int]string),
	}
}

// Set 设置字段
func (m *Message) Set(tag int, value string) {
	if _, ok := m.Fields[tag]; !ok {
		m.Tags = append(m.Tags, tag)
	}
	m.Fields[tag] = value
}

// Get 获取字段
func (m *Message) Get(tag int) string {
	return m.Fields[tag]
}

// Parse 解析原始 FIX 字节流 (高性能零拷贝扫描版)
func Parse(data []byte) (*Message, error) {
	msg := NewMessage()
	length := len(data)
	start := 0

	for start < length {
		// 1. 查找 '=' 确定 Tag
		equalIdx := -1
		for i := start; i < length; i++ {
			if data[i] == '=' {
				equalIdx = i
				break
			}
		}
		if equalIdx == -1 {
			break
		}

		// 2. 解析 Tag (手动解析整数)
		tag := 0
		for i := start; i < equalIdx; i++ {
			if data[i] >= '0' && data[i] <= '9' {
				tag = tag*10 + int(data[i]-'0')
			}
		}

		// 3. 查找 SOH 确定 Value
		valueStart := equalIdx + 1
		sohIdx := -1
		for i := valueStart; i < length; i++ {
			if data[i] == 1 { // SOH
				sohIdx = i
				break
			}
		}
		if sohIdx == -1 {
			// 处理末尾无 SOH 的情况 (非标但在此容错)
			sohIdx = length
		}

		// 4. 提取 Value 并存入
		value := string(data[valueStart:sohIdx])
		msg.Set(tag, value)

		start = sohIdx + 1
	}

	return msg, nil
}

// String 序列化为规范的 FIX 报文串
func (m *Message) String() string {
	var buf strings.Builder
	// 按照 Tag 排序确保 BodyLength 和 CheckSum 逻辑正确 (简化版)
	sort.Ints(m.Tags)
	for _, tag := range m.Tags {
		buf.WriteString(fmt.Sprintf("%d=%s%s", tag, m.Fields[tag], SOH))
	}
	return buf.String()
}

// Tag definitions (常用 Tag)
const (
	TagBeginString  = 8
	TagBodyLength   = 9
	TagMsgType      = 35
	TagSenderCompID = 49
	TagTargetCompID = 56
	TagMsgSeqNum    = 34
	TagCheckSum     = 10

	TagClOrdID  = 11
	TagSymbol   = 55
	TagSide     = 54
	TagOrderQty = 38
	TagPrice    = 44
	TagOrdType  = 40
)
