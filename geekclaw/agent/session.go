// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package agent

import (
	"github.com/seagosoft/geekclaw/geekclaw/interactive"
)

// sessionMode 会话模式常量类型。
type sessionMode int

const (
	modePico sessionMode = iota // 默认模式：消息 → LLM
	modeCmd                     // 命令模式：消息 → shell
)

// getSessionMode 获取指定会话的当前模式。
func (al *AgentLoop) getSessionMode(sessionKey string) sessionMode {
	if v, ok := al.sessionModes.Load(sessionKey); ok {
		return v
	}
	return modePico
}

// setSessionMode 设置指定会话的模式。
func (al *AgentLoop) setSessionMode(sessionKey string, mode sessionMode) {
	al.sessionModes.Store(sessionKey, mode)
}

// getSessionWorkDir 获取指定会话的工作目录。
func (al *AgentLoop) getSessionWorkDir(sessionKey string) string {
	if v, ok := al.sessionWorkDirs.Load(sessionKey); ok {
		return v
	}
	return ""
}

// setSessionWorkDir 设置指定会话的工作目录。
func (al *AgentLoop) setSessionWorkDir(sessionKey string, dir string) {
	al.sessionWorkDirs.Store(sessionKey, dir)
}

// getInteractiveMode 获取指定会话的交互模式。
func (al *AgentLoop) getInteractiveMode(sessionKey string) interactive.Mode {
	session := al.interactiveMgr.GetSession(sessionKey)
	if session == nil {
		return interactive.ModeAuto
	}
	return session.GetMode()
}

// setInteractiveMode 设置指定会话的交互模式。
func (al *AgentLoop) setInteractiveMode(sessionKey string, mode interactive.Mode) {
	session := al.interactiveMgr.GetSession(sessionKey)
	session.SetMode(mode)
}
