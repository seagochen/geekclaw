package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// jsonSession 用于迁移目的，对应 pkg/session.Session 结构。
type jsonSession struct {
	Key      string              `json:"key"`
	Messages []providers.Message `json:"messages"`
	Summary  string              `json:"summary,omitempty"`
	Created  time.Time           `json:"created"`
	Updated  time.Time           `json:"updated"`
}

// MigrateFromJSON 从 sessionsDir 读取旧版 sessions/*.json 文件，
// 将其写入 Store，并将每个已迁移的文件重命名为 .json.migrated 作为备份。
// 返回已迁移的会话数量。
//
// 无法解析的文件会被记录并跳过。已迁移的文件（.json.migrated）
// 将被忽略，使该函数具有幂等性。
func MigrateFromJSON(
	ctx context.Context, sessionsDir string, store Store,
) (int, error) {
	entries, err := os.ReadDir(sessionsDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("memory: read sessions dir: %w", err)
	}

	migrated := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		// 跳过 JSONL 元数据文件。它们属于新的存储格式，
		// 而非旧版会话快照，重新导入会用空消息列表
		// 覆盖配对的 .jsonl 历史记录。
		if strings.HasSuffix(name, ".meta.json") {
			continue
		}
		// 跳过已迁移的文件。
		if strings.HasSuffix(name, ".migrated") {
			continue
		}

		srcPath := filepath.Join(sessionsDir, name)

		data, readErr := os.ReadFile(srcPath)
		if readErr != nil {
			log.Printf("memory: migrate: skip %s: %v", name, readErr)
			continue
		}

		var sess jsonSession
		if parseErr := json.Unmarshal(data, &sess); parseErr != nil {
			log.Printf("memory: migrate: skip %s: %v", name, parseErr)
			continue
		}

		// 使用 JSON 内容中的键，而非文件名。
		// 文件名经过清理（":" -> "_"），但键未被清理。
		key := sess.Key
		if key == "" {
			key = strings.TrimSuffix(name, ".json")
		}

		// 使用 SetHistory（原子替换）而非逐条 AddFullMessage。
		// 这使迁移具有幂等性：如果进程在写入消息后但在
		// 下面的重命名之前崩溃，重试会干净地替换部分数据
		// 而不是重复消息。
		if setErr := store.SetHistory(ctx, key, sess.Messages); setErr != nil {
			return migrated, fmt.Errorf(
				"memory: migrate %s: set history: %w",
				name, setErr,
			)
		}

		if sess.Summary != "" {
			if sumErr := store.SetSummary(ctx, key, sess.Summary); sumErr != nil {
				return migrated, fmt.Errorf(
					"memory: migrate %s: set summary: %w",
					name, sumErr,
				)
			}
		}

		// 重命名为 .migrated 作为备份（不删除）。
		renameErr := os.Rename(srcPath, srcPath+".migrated")
		if renameErr != nil {
			log.Printf("memory: migrate: rename %s: %v", name, renameErr)
		}

		migrated++
	}

	return migrated, nil
}
