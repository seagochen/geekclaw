#!/usr/bin/env python3
"""
geekclaw CLI — Python 入口封装

用法:
    ./geekclaw.py <命令> [选项]

命令:
    agent      启动交互式 Agent
    version    显示版本信息

本脚本是对 Rust 二进制 `target/release/geekclaw` 的薄封装。
如果 release 二进制不存在，会自动尝试通过 cargo run 执行。
"""

import os
import subprocess
import sys
from pathlib import Path


def find_binary() -> list:
    """查找 geekclaw 二进制。优先 release，其次 debug，最后 cargo run。"""
    project_root = Path(__file__).resolve().parent

    # 1. release 二进制
    release = project_root / "target" / "release" / "geekclaw"
    if release.exists():
        return [str(release)]

    # 2. debug 二进制
    debug = project_root / "target" / "debug" / "geekclaw"
    if debug.exists():
        return [str(debug)]

    # 3. cargo run
    return ["cargo", "run", "--"]


def main():
    binary = find_binary()
    args = sys.argv[1:]

    # 传递环境变量
    env = os.environ.copy()

    # 配置文件路径
    config = env.get("GEEKCLAW_CONFIG")
    if not config:
        project_root = Path(__file__).resolve().parent
        for candidate in [
            project_root / "config.yaml",
            project_root / "geekclaw-app" / "configs" / "config.yaml",
        ]:
            if candidate.exists():
                config = str(candidate)
                break

    cmd = binary + args
    if config and "--config" not in args and "-c" not in args:
        cmd = binary + ["--config", config] + args

    try:
        result = subprocess.run(cmd, env=env)
        sys.exit(result.returncode)
    except KeyboardInterrupt:
        sys.exit(130)
    except FileNotFoundError:
        print("错误: 未找到 geekclaw 二进制。请先运行 ./manage.py build", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
