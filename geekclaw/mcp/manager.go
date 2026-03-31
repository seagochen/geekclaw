// Package mcp 管理与 MCP（Model Context Protocol）服务器的连接。
package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// headerTransport 是一个为请求添加自定义头部的 http.RoundTripper。
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 克隆请求以避免修改原始请求
	req = req.Clone(req.Context())

	// 添加自定义头部
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}

	// 使用基础传输
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// loadEnvFile 从 .env 格式的文件中加载环境变量。
// 每行格式为：KEY=value
// 以 # 开头的行为注释
// 空行将被忽略
func loadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	envVars := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 KEY=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("invalid format at line %d: empty key", lineNum)
		}

		// 如果存在引号则去除
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		envVars[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading env file: %w", err)
	}

	return envVars, nil
}

// ServerConnection 表示与 MCP 服务器的连接。
type ServerConnection struct {
	Name    string
	Client  *mcp.Client
	Session *mcp.ClientSession
	Tools   []*mcp.Tool
}

// Manager 管理多个 MCP 服务器连接。
type Manager struct {
	servers map[string]*ServerConnection
	mu      sync.RWMutex
	closed  atomic.Bool    // 从 bool 改为 atomic.Bool 以避免 TOCTOU 竞争
	wg      sync.WaitGroup // 追踪进行中的 CallTool 调用
}

// NewManager 创建一个新的 MCP 管理器。
func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]*ServerConnection),
	}
}

// LoadFromConfig 从配置中加载 MCP 服务器。
func (m *Manager) LoadFromConfig(ctx context.Context, cfg *config.Config) error {
	return m.LoadFromMCPConfig(ctx, cfg.Tools.MCP, cfg.WorkspacePath())
}

// LoadFromMCPConfig 从 MCP 配置和工作区路径加载 MCP 服务器。
// 这是不需要完整 Config 对象的最小依赖版本。
func (m *Manager) LoadFromMCPConfig(
	ctx context.Context,
	mcpCfg config.MCPConfig,
	workspacePath string,
) error {
	if !mcpCfg.Enabled {
		logger.InfoCF("mcp", "MCP integration is disabled", nil)
		return nil
	}

	if len(mcpCfg.Servers) == 0 {
		logger.InfoCF("mcp", "No MCP servers configured", nil)
		return nil
	}

	logger.InfoCF("mcp", "Initializing MCP servers",
		map[string]any{
			"count": len(mcpCfg.Servers),
		})

	var wg sync.WaitGroup
	errs := make(chan error, len(mcpCfg.Servers))
	enabledCount := 0

	for name, serverCfg := range mcpCfg.Servers {
		if !serverCfg.Enabled {
			logger.DebugCF("mcp", "Skipping disabled server",
				map[string]any{
					"server": name,
				})
			continue
		}

		enabledCount++
		wg.Add(1)
		go func(name string, serverCfg config.MCPServerConfig, workspace string) {
			defer wg.Done()

			// 将相对 envFile 路径解析为相对于工作区的路径
			if serverCfg.EnvFile != "" && !filepath.IsAbs(serverCfg.EnvFile) {
				if workspace == "" {
					err := fmt.Errorf(
						"workspace path is empty while resolving relative envFile %q for server %s",
						serverCfg.EnvFile,
						name,
					)
					logger.ErrorCF("mcp", "Invalid MCP server configuration",
						map[string]any{
							"server":   name,
							"env_file": serverCfg.EnvFile,
							"error":    err.Error(),
						})
					errs <- err
					return
				}
				serverCfg.EnvFile = filepath.Join(workspace, serverCfg.EnvFile)
			}

			if err := m.ConnectServer(ctx, name, serverCfg); err != nil {
				logger.ErrorCF("mcp", "Failed to connect to MCP server",
					map[string]any{
						"server": name,
						"error":  err.Error(),
					})
				errs <- fmt.Errorf("failed to connect to server %s: %w", name, err)
			}
		}(name, serverCfg, workspacePath)
	}

	wg.Wait()
	close(errs)

	// 收集错误
	var allErrors []error
	for err := range errs {
		allErrors = append(allErrors, err)
	}

	connectedCount := len(m.GetServers())

	// 如果所有已启用的服务器都连接失败，返回汇总错误
	if enabledCount > 0 && connectedCount == 0 {
		logger.ErrorCF("mcp", "All MCP servers failed to connect",
			map[string]any{
				"failed": len(allErrors),
				"total":  enabledCount,
			})
		return errors.Join(allErrors...)
	}

	if len(allErrors) > 0 {
		logger.WarnCF("mcp", "Some MCP servers failed to connect",
			map[string]any{
				"failed":    len(allErrors),
				"connected": connectedCount,
				"total":     enabledCount,
			})
		// 如果部分服务器成功连接，不完全失败
	}

	logger.InfoCF("mcp", "MCP server initialization complete",
		map[string]any{
			"connected": connectedCount,
			"total":     enabledCount,
		})

	return nil
}

// ConnectServer 连接到单个 MCP 服务器。
func (m *Manager) ConnectServer(
	ctx context.Context,
	name string,
	cfg config.MCPServerConfig,
) error {
	logger.InfoCF("mcp", "Connecting to MCP server",
		map[string]any{
			"server":     name,
			"command":    cfg.Command,
			"args_count": len(cfg.Args),
		})

	// 创建客户端
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "geekclaw",
		Version: "1.0.0",
	}, nil)

	// 根据配置创建传输层
	// 如果未明确指定，自动检测传输类型
	var transport mcp.Transport
	transportType := cfg.Type

	// 自动检测：如果提供了 URL，使用 SSE；如果提供了命令，使用 stdio
	if transportType == "" {
		if cfg.URL != "" {
			transportType = "sse"
		} else if cfg.Command != "" {
			transportType = "stdio"
		} else {
			return fmt.Errorf("either URL or command must be provided")
		}
	}

	switch transportType {
	case "sse", "http":
		if cfg.URL == "" {
			return fmt.Errorf("URL is required for SSE/HTTP transport")
		}
		logger.DebugCF("mcp", "Using SSE/HTTP transport",
			map[string]any{
				"server": name,
				"url":    cfg.URL,
			})

		sseTransport := &mcp.StreamableClientTransport{
			Endpoint: cfg.URL,
		}

		// 如果提供了自定义头部则添加
		if len(cfg.Headers) > 0 {
			// 创建带头部注入传输的自定义 HTTP 客户端
			sseTransport.HTTPClient = &http.Client{
				Transport: &headerTransport{
					base:    http.DefaultTransport,
					headers: cfg.Headers,
				},
			}
			logger.DebugCF("mcp", "Added custom HTTP headers",
				map[string]any{
					"server":       name,
					"header_count": len(cfg.Headers),
				})
		}

		transport = sseTransport
	case "stdio":
		if cfg.Command == "" {
			return fmt.Errorf("command is required for stdio transport")
		}
		logger.DebugCF("mcp", "Using stdio transport",
			map[string]any{
				"server":  name,
				"command": cfg.Command,
			})
		// 创建带上下文的命令
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)

		// 构建环境变量，使用正确的覆盖语义
		// 使用 map 确保配置变量覆盖文件变量
		envMap := make(map[string]string)

		// 从父进程环境开始
		for _, e := range cmd.Environ() {
			if idx := strings.Index(e, "="); idx > 0 {
				envMap[e[:idx]] = e[idx+1:]
			}
		}

		// 如果指定了文件，从文件加载环境变量
		if cfg.EnvFile != "" {
			envVars, err := loadEnvFile(cfg.EnvFile)
			if err != nil {
				return fmt.Errorf("failed to load env file %s: %w", cfg.EnvFile, err)
			}
			for k, v := range envVars {
				envMap[k] = v
			}
			logger.DebugCF("mcp", "Loaded environment variables from file",
				map[string]any{
					"server":    name,
					"envFile":   cfg.EnvFile,
					"var_count": len(envVars),
				})
		}

		// 配置中的环境变量覆盖文件中的
		for k, v := range cfg.Env {
			envMap[k] = v
		}

		// 将 map 转换为切片
		env := make([]string, 0, len(envMap))
		for k, v := range envMap {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env

		transport = &mcp.CommandTransport{Command: cmd}
	default:
		return fmt.Errorf(
			"unsupported transport type: %s (supported: stdio, sse, http)",
			transportType,
		)
	}

	// 连接到服务器
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// 获取服务器信息
	initResult := session.InitializeResult()
	logger.InfoCF("mcp", "Connected to MCP server",
		map[string]any{
			"server":        name,
			"serverName":    initResult.ServerInfo.Name,
			"serverVersion": initResult.ServerInfo.Version,
			"protocol":      initResult.ProtocolVersion,
		})

	// 如果支持，列出可用工具
	var tools []*mcp.Tool
	if initResult.Capabilities.Tools != nil {
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				logger.WarnCF("mcp", "Error listing tool",
					map[string]any{
						"server": name,
						"error":  err.Error(),
					})
				continue
			}
			tools = append(tools, tool)
		}

		logger.InfoCF("mcp", "Listed tools from MCP server",
			map[string]any{
				"server":    name,
				"toolCount": len(tools),
			})
	}

	// 存储连接
	m.mu.Lock()
	m.servers[name] = &ServerConnection{
		Name:    name,
		Client:  client,
		Session: session,
		Tools:   tools,
	}
	m.mu.Unlock()

	return nil
}

// GetServers 返回所有已连接的服务器。
func (m *Manager) GetServers() map[string]*ServerConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ServerConnection, len(m.servers))
	for k, v := range m.servers {
		result[k] = v
	}
	return result
}

// GetServer 返回指定名称的服务器连接。
func (m *Manager) GetServer(name string) (*ServerConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, ok := m.servers[name]
	return conn, ok
}

// CallTool 在指定服务器上调用工具。
func (m *Manager) CallTool(
	ctx context.Context,
	serverName, toolName string,
	arguments map[string]any,
) (*mcp.CallToolResult, error) {
	// 获取锁前检查是否已关闭（快速路径）
	if m.closed.Load() {
		return nil, fmt.Errorf("manager is closed")
	}

	m.mu.RLock()
	// 获取锁后二次检查，防止 TOCTOU 竞争
	if m.closed.Load() {
		m.mu.RUnlock()
		return nil, fmt.Errorf("manager is closed")
	}
	conn, ok := m.servers[serverName]
	if ok {
		m.wg.Add(1) // 持有锁时添加到 WaitGroup
	}
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("server %s not found", serverName)
	}
	defer m.wg.Done()

	params := &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	}

	result, err := conn.Session.CallTool(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}

	return result, nil
}

// Close 关闭所有服务器连接。
func (m *Manager) Close() error {
	// 使用 Swap 原子地设置 closed=true 并获取之前的值
	// 防止与 CallTool 的 closed 检查发生 TOCTOU 竞争
	if m.closed.Swap(true) {
		return nil // 已经关闭
	}

	// 等待所有进行中的 CallTool 调用完成后再关闭会话
	// 设置 closed=true 后，新的 CallTool 无法启动（它们会先检查 closed）
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	logger.InfoCF("mcp", "Closing all MCP server connections",
		map[string]any{
			"count": len(m.servers),
		})

	var errs []error
	for name, conn := range m.servers {
		if err := conn.Session.Close(); err != nil {
			logger.ErrorCF("mcp", "Failed to close server connection",
				map[string]any{
					"server": name,
					"error":  err.Error(),
				})
			errs = append(errs, fmt.Errorf("server %s: %w", name, err))
		}
	}

	m.servers = make(map[string]*ServerConnection)

	if len(errs) > 0 {
		return fmt.Errorf("failed to close %d server(s): %w", len(errs), errors.Join(errs...))
	}

	return nil
}

// GetAllTools 返回所有已连接服务器的所有工具。
func (m *Manager) GetAllTools() map[string][]*mcp.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]*mcp.Tool)
	for name, conn := range m.servers {
		if len(conn.Tools) > 0 {
			result[name] = conn.Tools
		}
	}
	return result
}
