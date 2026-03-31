package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// SpawnOpts 配置插件进程的启动方式。
type SpawnOpts struct {
	LogCategory    string                                   // 日志分类（例如 "commands"、"search"）
	InitMethod     string                                   // 例如 "command.initialize"
	InitParams     any                                      // 初始化握手的参数
	StopMethod     string                                   // 例如 "command.stop"
	LogMethod      string                                   // 例如 "command.log"
	OnNotification func(context.Context, *Notification)     // 非日志通知的处理函数
	Services       map[string]ServiceHandler                // 反向调用：插件可调用的 Go 服务
}

// Process 管理通过 stdio 上的 JSON-RPC 通信的外部插件进程的生命周期。
// 处理进程启动、初始化握手、通知路由和优雅关闭。
//
// 典型用法：
//
//	proc := plugin.NewProcess("myplugin", cfg)
//	raw, err := proc.Spawn(ctx, opts)
//	// 解析原始初始化结果...
//	// 使用 proc.Transport().Call(...) 进行领域特定的 RPC 调用
//	// 完成后调用 proc.Stop()
type Process struct {
	name       string
	cfg        Config
	cmd        *exec.Cmd
	transport  *Transport
	cancel     context.CancelFunc
	stopMethod string
	logMethod  string
	logCat     string
	wg         sync.WaitGroup
	mu         sync.Mutex
}

// NewProcess 为给定的插件名称和配置创建一个新的 Process。
func NewProcess(name string, cfg Config) *Process {
	return &Process{name: name, cfg: cfg}
}

// Name 返回插件名称。
func (p *Process) Name() string { return p.name }

// Transport 返回底层的 JSON-RPC 传输层。仅在 Spawn 后有效。
func (p *Process) Transport() *Transport { return p.transport }

// PluginConfig 返回插件配置。
func (p *Process) PluginConfig() Config { return p.cfg }

// RegisterService 注册一个 Go 服务处理函数，供插件反向调用。
// 必须在 Spawn 之前调用，或通过 SpawnOpts.Services 传入。
func (p *Process) RegisterService(method string, handler ServiceHandler) {
	if p.transport != nil {
		p.transport.RegisterService(method, handler)
	}
}

// Spawn 启动外部进程，创建传输层，启动读取循环和通知处理协程，
// 执行初始化握手，并返回初始化调用的原始 JSON 结果。
//
// 失败时，进程会自动清理。
func (p *Process) Spawn(ctx context.Context, opts SpawnOpts) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cfg.Command == "" {
		return nil, fmt.Errorf("plugin %q: command is required", p.name)
	}

	p.stopMethod = opts.StopMethod
	p.logMethod = opts.LogMethod
	p.logCat = opts.LogCategory

	procCtx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	args := append([]string{}, p.cfg.Args...)
	cmd := exec.CommandContext(procCtx, p.cfg.Command, args...)
	// 构建子进程环境：继承当前环境，用 cfg.Env 覆盖（而非追加）同名变量
	overrides := make(map[string]string, len(p.cfg.Env))
	for k, v := range p.cfg.Env {
		overrides[k] = v
	}
	baseEnv := os.Environ()
	merged := make([]string, 0, len(baseEnv)+len(overrides))
	for _, entry := range baseEnv {
		key := entry
		if i := strings.Index(entry, "="); i >= 0 {
			key = entry[:i]
		}
		if _, ok := overrides[key]; !ok {
			merged = append(merged, entry)
		}
	}
	for k, v := range overrides {
		merged = append(merged, k+"="+v)
	}
	cmd.Env = merged
	cmd.Stderr = &LogWriter{Name: p.name, Category: opts.LogCategory}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start command %q: %w", p.cfg.Command, err)
	}

	p.cmd = cmd
	p.transport = NewTransport(stdout, stdin)
	p.transport.SetContext(procCtx)

	// 注册反向调用服务处理函数
	for method, handler := range opts.Services {
		p.transport.RegisterService(method, handler)
	}

	// 启动读取循环
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.transport.ReadLoop(); err != nil {
			logger.WarnCF(opts.LogCategory, "Plugin read loop ended", map[string]any{
				"plugin": p.name,
				"error":  err.Error(),
			})
		}
	}()

	// 启动通知处理
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.handleNotifications(procCtx, opts)
	}()

	// 初始化握手
	raw, err := p.transport.Call(ctx, opts.InitMethod, opts.InitParams)
	if err != nil {
		p.stopLocked()
		return nil, fmt.Errorf("initialize handshake failed: %w", err)
	}

	return raw, nil
}

// Stop 优雅地关闭插件进程。
func (p *Process) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

// stopLocked 在持有锁的情况下停止进程。
func (p *Process) stopLocked() {
	if p.cancel != nil {
		if p.transport != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5e9) // 5 秒
			_, _ = p.transport.Call(stopCtx, p.stopMethod, nil)
			cancel()
		}
		p.cancel()
		p.cancel = nil
	}

	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Wait()
		p.cmd = nil
	}

	p.wg.Wait()
}

// handleNotifications 处理来自插件的通知。
func (p *Process) handleNotifications(ctx context.Context, opts SpawnOpts) {
	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok := <-p.transport.Notifications():
			if !ok {
				return
			}
			if notif.Method == opts.LogMethod {
				HandleLogNotification(notif, p.name, opts.LogCategory)
			} else if opts.OnNotification != nil {
				opts.OnNotification(ctx, notif)
			}
		}
	}
}
