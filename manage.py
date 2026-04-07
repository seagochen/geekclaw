#!/usr/bin/env python3
"""
geekclaw 构建脚本 (开发用)

用法:
    ./manage.py <命令> [选项]

命令:
    build                    使用 cargo 编译 release 二进制
    clean                    删除构建产物
    test                     运行所有测试
    bench                    运行性能基准测试
"""

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path


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


def run(cmd: list, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, check=check)


# ---------------------------------------------------------------------------
# 命令
# ---------------------------------------------------------------------------

BINARY_NAME = "geekclaw"


class GeekclawBuilder:

    def __init__(self, project_root: Path):
        self.project_root = project_root
        self.target_dir = project_root / "target"
        self.release_binary = self.target_dir / "release" / BINARY_NAME

    def cmd_build(self):
        section("编译 (release)")
        run(["cargo", "build", "--release"])
        if self.release_binary.exists():
            size_kb = self.release_binary.stat().st_size // 1024
            info(f"编译完成: {self.release_binary}  ({size_kb} KB)")
        else:
            error("编译失败: 未找到二进制文件")
            sys.exit(1)

        section("准备运行环境")
        self._setup_config()

        print()
        print("🦞 geekclaw is ready!")
        print()
        print("使用方法:")
        print(f"  {self.release_binary} version     # 显示版本")
        print(f"  {self.release_binary} agent       # 启动交互式 Agent")
        print()
        print("或者通过 cargo:")
        print("  cargo run -- agent")

    def _setup_config(self):
        """如果 config.yaml 不存在，从 config.example.yaml 复制一份。"""
        config_path = self.project_root / "config.yaml"
        example_path = self.project_root / "config.example.yaml"

        if config_path.exists():
            info(f"配置文件已存在，跳过: {config_path}")
        elif example_path.exists():
            shutil.copy2(example_path, config_path)
            info(f"已从模板创建配置文件: {config_path}")
            info("请编辑 config.yaml 填写 API 密钥")
        else:
            warn("未找到 config.example.yaml，跳过配置文件创建")

    def cmd_clean(self):
        section("清理")
        run(["cargo", "clean"])
        info("已清理构建产物")

    def cmd_test(self):
        section("运行测试")
        run(["cargo", "test"])

    def cmd_bench(self):
        section("运行基准测试")
        run(["cargo", "bench", "-p", "geekclaw-memory", "--bench", "jsonl_bench"])


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

HELP = """\
geekclaw 构建脚本

用法: ./manage.py <命令>

命令:
  build    编译 release 二进制
  clean    删除构建产物
  test     运行所有测试
  bench    运行性能基准测试

典型工作流:
  ./manage.py build                    # 编译
  export OPENAI_API_KEY=sk-...         # 设置 API 密钥
  ./target/release/geekclaw agent      # 启动 Agent
"""


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
    subparsers.add_parser("test",  add_help=False)
    subparsers.add_parser("bench", add_help=False)

    args = parser.parse_args()

    if args.help or args.command is None:
        print(HELP)
        sys.exit(0)

    builder = GeekclawBuilder(project_root)

    commands = {
        "build": builder.cmd_build,
        "clean": builder.cmd_clean,
        "test":  builder.cmd_test,
        "bench": builder.cmd_bench,
    }

    if args.command in commands:
        commands[args.command]()
    else:
        error(f"未知命令: {args.command}")
        print("运行 './manage.py --help' 查看帮助")
        sys.exit(1)


if __name__ == "__main__":
    main()
