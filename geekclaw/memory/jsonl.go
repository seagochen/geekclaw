// Package memory 提供基于 JSONL 文件的会话持久化存储。
package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/fileutil"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

const (
	// numLockShards 是用于序列化每会话访问的固定互斥锁数量。
	// 使用分片数组而非 map，确保内存使用不随会话总数增长 —
	// 对长期运行的守护进程非常重要。
	numLockShards = 64

	// maxLineSize 是 .jsonl 文件中单行 JSON 的最大大小。
	// 工具结果（read_file、web search 等）可能很大，因此
	// 设置了较宽松的限制。scanner 从 64 KB 开始，按需增长
	// 至此上限。
	maxLineSize = 10 * 1024 * 1024 // 10 MB
)

// sessionMeta 保存存储在 .meta.json 文件中的每会话元数据。
type sessionMeta struct {
	Key       string    `json:"key"`
	Summary   string    `json:"summary"`
	Skip      int       `json:"skip"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JSONLStore 使用追加式 JSONL 文件实现 Store 接口。
//
// 每个会话存储为两个文件：
//
//	{sanitized_key}.jsonl      — 每行一条 JSON 编码的消息，仅追加
//	{sanitized_key}.meta.json  — 会话元数据（摘要、逻辑截断偏移量）
//
// 消息不会从 JSONL 文件中物理删除。TruncateHistory 在元数据文件中
// 记录 "skip" 偏移量，GetHistory 忽略该偏移量之前的行。这使所有写入
// 都是追加式的，既快速又具有崩溃安全性。
type JSONLStore struct {
	dir     string
	metaDir string
	locks   [numLockShards]sync.Mutex

	// 元数据内存缓存，避免每次操作都读磁盘
	metaCacheMu sync.RWMutex
	metaCache   map[string]sessionMeta
}

// NewJSONLStore 创建一个新的 JSONL 存储。
// 会话消息（.jsonl）存储在 dir 中，元数据
// （.meta.json）存储在 metaDir 中，使运行时状态
// 位于 logs/ 目录树下。
func NewJSONLStore(dir, metaDir string) (*JSONLStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("memory: create directory: %w", err)
	}
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return nil, fmt.Errorf("memory: create meta directory: %w", err)
	}
	return &JSONLStore{dir: dir, metaDir: metaDir, metaCache: make(map[string]sessionMeta)}, nil
}

// sessionLock 返回给定会话键的互斥锁。
// 通过 FNV 哈希将键映射到固定的分片池，因此
// 无论会话总数如何，内存使用都是 O(1)。
func (s *JSONLStore) sessionLock(key string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(key))
	return &s.locks[h.Sum32()%numLockShards]
}

func (s *JSONLStore) jsonlPath(key string) string {
	return filepath.Join(s.dir, sanitizeKey(key)+".jsonl")
}

func (s *JSONLStore) metaPath(key string) string {
	return filepath.Join(s.metaDir, sanitizeKey(key)+".meta.json")
}

// sanitizeKey 将会话键转换为安全的文件名组件。
// 与 pkg/session.sanitizeFilename 保持一致，以便迁移路径匹配。
// 将 ':' 替换为 '_'（会话键分隔符），将 '/' 和 '\' 替换为 '_'，
// 以确保复合 ID（如 Telegram 论坛的 "chatID/threadID"、Slack 的 "channel/thread_ts"）
// 不会创建子目录或在 Windows 上出错。
func sanitizeKey(key string) string {
	s := strings.ReplaceAll(key, ":", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

// readMeta 加载会话的元数据，优先从内存缓存读取。
// 如果缓存未命中，从磁盘加载并缓存结果。
func (s *JSONLStore) readMeta(key string) (sessionMeta, error) {
	// 先查缓存
	s.metaCacheMu.RLock()
	if cached, ok := s.metaCache[key]; ok {
		s.metaCacheMu.RUnlock()
		return cached, nil
	}
	s.metaCacheMu.RUnlock()

	// 缓存未命中，从磁盘读取
	data, err := os.ReadFile(s.metaPath(key))
	if os.IsNotExist(err) {
		meta := sessionMeta{Key: key}
		s.metaCacheMu.Lock()
		s.metaCache[key] = meta
		s.metaCacheMu.Unlock()
		return meta, nil
	}
	if err != nil {
		return sessionMeta{}, fmt.Errorf("memory: read meta: %w", err)
	}
	var meta sessionMeta
	err = json.Unmarshal(data, &meta)
	if err != nil {
		return sessionMeta{}, fmt.Errorf("memory: decode meta: %w", err)
	}

	s.metaCacheMu.Lock()
	s.metaCache[key] = meta
	s.metaCacheMu.Unlock()

	return meta, nil
}

// writeMeta 使用项目标准的 WriteFileAtomic（临时文件 + fsync + 重命名）
// 原子地写入元数据文件，并更新内存缓存。
func (s *JSONLStore) writeMeta(key string, meta sessionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("memory: encode meta: %w", err)
	}
	if err := fileutil.WriteFileAtomic(s.metaPath(key), data, 0o644); err != nil {
		return err
	}

	// 写入成功后更新缓存
	s.metaCacheMu.Lock()
	s.metaCache[key] = meta
	s.metaCacheMu.Unlock()

	return nil
}

// readMessages 从 .jsonl 文件读取有效的 JSON 行，跳过
// 前 `skip` 行而不进行反序列化。这避免了对逻辑截断消息
// 执行 json.Unmarshal 的开销。
// 格式错误的尾部行（例如崩溃导致的）将被静默跳过。
func readMessages(path string, skip int) ([]providers.Message, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []providers.Message{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: open jsonl: %w", err)
	}
	defer f.Close()

	var msgs []providers.Message
	scanner := bufio.NewScanner(f)
	// 允许较大的行以容纳工具结果（read_file、web search 等）。
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lineNum++
		if lineNum <= skip {
			continue
		}
		var msg providers.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			// 损坏的行 — 可能是崩溃导致的不完整写入。
			// 记录日志以便运维人员知道数据被跳过，但不
			// 使整个读取失败；这是标准的 JSONL 恢复模式。
			log.Printf("memory: skipping corrupt line %d in %s: %v",
				lineNum, filepath.Base(path), err)
			continue
		}
		msgs = append(msgs, msg)
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("memory: scan jsonl: %w", scanner.Err())
	}

	if msgs == nil {
		msgs = []providers.Message{}
	}
	return msgs, nil
}

// countLines 统计 .jsonl 文件中非空行的总数。
// 供 TruncateHistory 使用，无需反序列化每条消息
// 即可校正陈旧的 meta.Count。
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("memory: open jsonl: %w", err)
	}
	defer f.Close()

	n := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			n++
		}
	}
	return n, scanner.Err()
}

// AddMessage 向会话追加一条简单文本消息。
func (s *JSONLStore) AddMessage(
	_ context.Context, sessionKey, role, content string,
) error {
	return s.addMsg(sessionKey, providers.Message{
		Role:    role,
		Content: content,
	})
}

// AddFullMessage 向会话追加一条完整消息（包含工具调用等）。
func (s *JSONLStore) AddFullMessage(
	_ context.Context, sessionKey string, msg providers.Message,
) error {
	return s.addMsg(sessionKey, msg)
}

// addMsg 是 AddMessage 和 AddFullMessage 的共享实现。
func (s *JSONLStore) addMsg(sessionKey string, msg providers.Message) error {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	// 将消息作为单行 JSON 追加。
	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("memory: marshal message: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(
		s.jsonlPath(sessionKey),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("memory: open jsonl for append: %w", err)
	}
	_, writeErr := f.Write(line)
	if writeErr != nil {
		f.Close()
		return fmt.Errorf("memory: append message: %w", writeErr)
	}
	// 关闭前刷新到物理存储。这与 writeMeta 和 rewriteJSONL
	// （使用带 fsync 的 WriteFileAtomic）的持久性保证一致。
	// 没有 Sync，断电可能导致追加仅留在内核页缓存中 — 重启后丢失。
	if syncErr := f.Sync(); syncErr != nil {
		f.Close()
		return fmt.Errorf("memory: sync jsonl: %w", syncErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		return fmt.Errorf("memory: close jsonl: %w", closeErr)
	}

	// 更新元数据。
	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return err
	}
	now := time.Now()
	if meta.Count == 0 && meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.Count++
	meta.UpdatedAt = now

	return s.writeMeta(sessionKey, meta)
}

// GetHistory 返回会话的所有消息，按插入顺序排列。
func (s *JSONLStore) GetHistory(
	_ context.Context, sessionKey string,
) ([]providers.Message, error) {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return nil, err
	}

	// 传递 meta.Skip 使 readMessages 跳过这些行而不
	// 反序列化 — 避免在截断消息上浪费 CPU。
	msgs, err := readMessages(s.jsonlPath(sessionKey), meta.Skip)
	if err != nil {
		return nil, err
	}

	return msgs, nil
}

// GetSummary 返回会话的对话摘要。
func (s *JSONLStore) GetSummary(
	_ context.Context, sessionKey string,
) (string, error) {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return "", err
	}
	return meta.Summary, nil
}

// SetSummary 更新会话的对话摘要。
func (s *JSONLStore) SetSummary(
	_ context.Context, sessionKey, summary string,
) error {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return err
	}
	now := time.Now()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.Summary = summary
	meta.UpdatedAt = now

	return s.writeMeta(sessionKey, meta)
}

// TruncateHistory 移除除最后 keepLast 条之外的所有消息。
func (s *JSONLStore) TruncateHistory(
	_ context.Context, sessionKey string, keepLast int,
) error {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return err
	}

	// 始终将 meta.Count 与磁盘上的实际行数进行校正。
	// addMsg 中 JSONL 追加和元数据更新之间的崩溃
	// 会导致 meta.Count 过时（例如文件有 101 行但 meta 显示
	// 100）。行计数开销很低 — 无需反序列化，只需扫描 —
	// 且 TruncateHistory 不是热路径，因此始终重新计数。
	n, countErr := countLines(s.jsonlPath(sessionKey))
	if countErr != nil {
		return countErr
	}
	meta.Count = n

	if keepLast <= 0 {
		meta.Skip = meta.Count
	} else {
		effective := meta.Count - meta.Skip
		if keepLast < effective {
			meta.Skip = meta.Count - keepLast
		}
	}
	meta.UpdatedAt = time.Now()

	return s.writeMeta(sessionKey, meta)
}

// SetHistory 用提供的历史记录替换会话中的所有消息。
func (s *JSONLStore) SetHistory(
	_ context.Context,
	sessionKey string,
	history []providers.Message,
) error {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return err
	}
	now := time.Now()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.Skip = 0
	meta.Count = len(history)
	meta.UpdatedAt = now

	// 在重写 JSONL 文件之前先写入元数据。如果两次写入之间
	// 发生崩溃，meta 中 Skip=0 且旧文件仍完整，
	// 所以 GetHistory 从第 1 行读取 — 返回"过多"消息
	// 而非丢失数据。下次 SetHistory 调用会修正这个问题。
	err = s.writeMeta(sessionKey, meta)
	if err != nil {
		return err
	}

	return s.rewriteJSONL(sessionKey, history)
}

// Compact 物理重写 JSONL 文件，丢弃所有逻辑上已跳过的行。
// 这会回收 TruncateHistory 反复调用后累积的磁盘空间。
//
// 可以随时安全调用；如果没有需要压缩的内容（skip == 0），
// 方法立即返回。
func (s *JSONLStore) Compact(
	_ context.Context, sessionKey string,
) error {
	l := s.sessionLock(sessionKey)
	l.Lock()
	defer l.Unlock()

	meta, err := s.readMeta(sessionKey)
	if err != nil {
		return err
	}
	if meta.Skip == 0 {
		return nil
	}

	// 只读取活跃的消息，跳过截断的行而不进行反序列化。
	active, err := readMessages(s.jsonlPath(sessionKey), meta.Skip)
	if err != nil {
		return err
	}

	// 在重写 JSONL 文件之前先写入元数据。如果进程
	// 在两次写入之间崩溃，meta 中 Skip=0 且旧的
	// （未压缩的）文件仍完整，所以 GetHistory 从
	// 第 1 行读取 — 返回先前截断的消息而非丢失数据。
	// 下次 Compact 或 TruncateHistory 会修正这个问题。
	meta.Skip = 0
	meta.Count = len(active)
	meta.UpdatedAt = time.Now()

	err = s.writeMeta(sessionKey, meta)
	if err != nil {
		return err
	}

	return s.rewriteJSONL(sessionKey, active)
}

// rewriteJSONL 使用项目标准的 WriteFileAtomic（临时文件 + fsync + 重命名）
// 原子地替换 JSONL 文件。
func (s *JSONLStore) rewriteJSONL(
	sessionKey string, msgs []providers.Message,
) error {
	var buf bytes.Buffer
	for i, msg := range msgs {
		line, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("memory: marshal message %d: %w", i, err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return fileutil.WriteFileAtomic(s.jsonlPath(sessionKey), buf.Bytes(), 0o644)
}

// Close 释放存储持有的所有资源。
func (s *JSONLStore) Close() error {
	return nil
}
