#!/usr/bin/env python3
"""
geekclaw 构建脚本 (开发用)
发布构建请使用: goreleaser release

用法:
    ./manage.py <命令> [选项]

命令:
    build                    生成并编译，然后准备 geekclaw-app/ 运行环境
    install                  安装到 $GEEKCLAW_HOME（系统级部署）
    install --user           创建专用 Linux 用户 geekclaw 并安装
    uninstall                删除 $GEEKCLAW_HOME
    uninstall --user         删除专用 Linux 用户及主目录
    clean                    删除构建产物和 geekclaw-app/
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional


# ---------------------------------------------------------------------------
# 颜色
# ---------------------------------------------------------------------------

class Colors:
    RED    = '\033[0;31m'
    GREEN  = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE   = '\033[0;34m'
    NC     = '\033[0m'

    @classmethod
    def disable(cls):
        cls.RED = cls.GREEN = cls.YELLOW = cls.BLUE = cls.NC = ''


def info(msg):    print(f"{Colors.GREEN}[INFO]{Colors.NC} {msg}")
def warn(msg):    print(f"{Colors.YELLOW}[WARN]{Colors.NC} {msg}")
def error(msg):   print(f"{Colors.RED}[ERROR]{Colors.NC} {msg}", file=sys.stderr)
def section(msg): print(f"\n{Colors.BLUE}===== {msg} ====={Colors.NC}")


# ---------------------------------------------------------------------------
# 工具函数
# ---------------------------------------------------------------------------

def run(cmd: list, env: Optional[dict] = None, check: bool = True) -> subprocess.CompletedProcess:
    merged_env = {**os.environ, **(env or {})}
    return subprocess.run(cmd, env=merged_env, check=check)


def capture(cmd: list, default: str = "") -> str:
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return result.stdout.strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return default


# ---------------------------------------------------------------------------
# 构建逻辑
# ---------------------------------------------------------------------------

BINARY_NAME = "geekclaw-cli"
BUILD_DIR   = "build"
CMD_DIR     = "geekclaw"       # Go 源码目录
APP_DIR     = "geekclaw-app"   # 本地运行环境目录
CONFIG_PKG  = "github.com/seagosoft/geekclaw/geekclaw/config"


class GeekclawBuilder:

    def __init__(self, project_root: Path):
        self.project_root = project_root
        self.build_dir    = project_root / BUILD_DIR
        self.binary       = self.build_dir / BINARY_NAME
        self.app_dir      = project_root / APP_DIR

    # ------------------------------------------------------------------ #
    # 版本信息
    # ------------------------------------------------------------------ #

    def _version_info(self) -> dict:
        version    = capture(["git", "describe", "--tags", "--always", "--dirty"], default="dev")
        git_commit = capture(["git", "rev-parse", "--short=8", "HEAD"], default="dev")
        build_time = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S%z")
        go_version = capture(["go", "version"], default="")
        go_ver = go_version.split()[2] if len(go_version.split()) >= 3 else "unknown"
        return {
            "version":    version,
            "git_commit": git_commit,
            "build_time": build_time,
            "go_version": go_ver,
        }

    def _ldflags(self, v: dict) -> str:
        flags = [
            f"-X {CONFIG_PKG}.Version={v['version']}",
            f"-X {CONFIG_PKG}.GitCommit={v['git_commit']}",
            f"-X {CONFIG_PKG}.BuildTime={v['build_time']}",
            f"-X {CONFIG_PKG}.GoVersion={v['go_version']}",
            "-s", "-w",
        ]
        return " ".join(flags)

    # ------------------------------------------------------------------ #
    # build
    # ------------------------------------------------------------------ #

    def cmd_build(self):
        section("生成")
        run(["go", "generate", "./..."])
        info("go generate 完成")

        section("编译")
        v = self._version_info()
        info(f"版本:     {v['version']}")
        info(f"提交:     {v['git_commit']}")
        info(f"构建时间: {v['build_time']}")
        info(f"Go 版本:  {v['go_version']}")
        info(f"目标:     {self.build_dir / BINARY_NAME}")
        print()

        self.build_dir.mkdir(parents=True, exist_ok=True)
        run(
            ["go", "build", "-tags", "stdjson",
             "-ldflags", self._ldflags(v),
             "-o", str(self.binary),
             f"./{CMD_DIR}"],
            env={"CGO_ENABLED": "0"},
        )
        size_kb = self.binary.stat().st_size // 1024
        info(f"编译完成: {self.binary}  ({size_kb} KB)")

        section("准备运行环境")
        self._setup_env(self.app_dir)

    # ------------------------------------------------------------------ #
    # 环境准备（内部）
    # ------------------------------------------------------------------ #

    def _setup_env(self, home: Path):
        workspace   = home / "workspace"
        plugins_dir = home / "plugins"
        config_path = home / "configs" / "config.yaml"

        # 创建目录结构
        for d in [home / "bin", home / "configs", home / "logs", workspace, plugins_dir]:
            d.mkdir(parents=True, exist_ok=True)

        # 复制二进制
        dst_bin = home / "bin" / BINARY_NAME
        shutil.copy2(self.binary, dst_bin)
        dst_bin.chmod(0o755)

        # 配置文件
        self._handle_config(config_path, workspace, plugins_dir)

        # 复制插件子目录（本工程 plugins/）
        for subdir in ("persona", "memory", "skills"):
            src = self.project_root / "plugins" / subdir
            if src.is_dir():
                dst = plugins_dir / subdir
                if dst.exists():
                    shutil.rmtree(dst)
                shutil.copytree(src, dst)

        print()
        print("🦞 geekclaw is ready!")
        print()
        print("Next steps:")
        print(f"  1. Edit config:  {config_path}")
        print()
        print("  2. Run:")
        print("       ./geekclaw.py gateway    # 启动 gateway 模式（接入 Telegram / Discord 等）")
        print("       ./geekclaw.py agent      # 直接进入 agent 交互模式")

    def _handle_config(self, config_path: Path, workspace: Path, plugins_dir: Path):
        config_src = self.project_root / "templates" / "configs" / "config.example.yaml"

        if not config_path.exists():
            content = config_src.read_text()
            content = content.replace("workspace: ~/.geekclaw/workspace", f"workspace: {workspace}")
            content = content.replace("plugins_dir: ~/.geekclaw/plugins",  f"plugins_dir: {plugins_dir}")
            config_path.write_text(content)
            info(f"Config written: {config_path}")
        elif re.search(r'^\s*restrict_to_plugins_dir:', config_path.read_text(), re.MULTILINE):
            # 迁移旧配置
            content = config_path.read_text()
            content = re.sub(
                r'(\s*plugins_dir:)',
                f'    workspace: {workspace}\n\\1',
                content, count=1,
            )
            content = re.sub(
                r'(\s*)restrict_to_plugins_dir:',
                r'\1restrict_to_workspace:',
                content,
            )
            config_path.write_text(content)
            info(f"Config migrated: {config_path}")
        else:
            info(f"Config exists, skipped: {config_path}")

    # ------------------------------------------------------------------ #
    # clean
    # ------------------------------------------------------------------ #

    def cmd_clean(self):
        if self.build_dir.exists():
            shutil.rmtree(self.build_dir)
            info(f"已删除: {self.build_dir}")
        run(["go", "clean", "-cache"])
        if self.app_dir.exists():
            shutil.rmtree(self.app_dir)
            info(f"已删除: {self.app_dir}")


# ---------------------------------------------------------------------------
# 帮助信息
# ---------------------------------------------------------------------------

HELP = """\
geekclaw 构建脚本（发布构建请使用 goreleaser）

用法: ./manage.py <命令>

命令:
  build    生成并编译，准备 geekclaw-app/ 运行环境
  clean    删除构建产物和 geekclaw-app/

典型工作流:
  ./manage.py build                    # 编译 + 准备 geekclaw-app/
  vi geekclaw-app/configs/config.yaml  # 填写 API 密钥
  ./geekclaw.py gateway                # 启动服务
"""


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main():
    project_root = Path(__file__).resolve().parent

    if not sys.stdout.isatty():
        Colors.disable()

    parser = argparse.ArgumentParser(
        description="geekclaw 构建脚本",
        add_help=False,
    )
    parser.add_argument("-h", "--help", action="store_true")

    subparsers = parser.add_subparsers(dest="command")
    subparsers.add_parser("build", add_help=False)
    subparsers.add_parser("clean", add_help=False)

    args = parser.parse_args()

    if args.help or args.command is None:
        print(HELP)
        sys.exit(0)

    builder = GeekclawBuilder(project_root)

    if args.command == "build":
        builder.cmd_build()
    elif args.command == "clean":
        builder.cmd_clean()
    else:
        error(f"未知命令: {args.command}")
        print("运行 './manage.py --help' 查看帮助")
        sys.exit(1)


if __name__ == "__main__":
    main()
