// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package config

import (
	"fmt"
	"runtime"
)

// 构建时通过 ldflags 注入的变量。
// 由 Makefile 或 .goreleaser.yaml 使用 -X 标志设置：
//
//	-X github.com/seagosoft/geekclaw/pkg/config.Version=<version>
//	-X github.com/seagosoft/geekclaw/pkg/config.GitCommit=<commit>
//	-X github.com/seagosoft/geekclaw/pkg/config.BuildTime=<timestamp>
//	-X github.com/seagosoft/geekclaw/pkg/config.GoVersion=<go-version>
var (
	Version   = "dev" // 未使用 ldflags 构建时的默认值
	GitCommit string  // Git 提交 SHA（缩写）
	BuildTime string  // 构建时间戳，RFC3339 格式
	GoVersion string  // 构建使用的 Go 版本
)

// FormatVersion 返回版本字符串，可选附带 git 提交信息。
func FormatVersion() string {
	v := Version
	if GitCommit != "" {
		v += fmt.Sprintf(" (git: %s)", GitCommit)
	}
	return v
}

// FormatBuildInfo 返回构建时间和 Go 版本信息。
func FormatBuildInfo() (string, string) {
	build := BuildTime
	goVer := GoVersion
	if goVer == "" {
		goVer = runtime.Version()
	}
	return build, goVer
}

// GetVersion 返回版本字符串。
func GetVersion() string {
	return Version
}
