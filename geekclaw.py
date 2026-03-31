#!/usr/bin/env python3
"""
geekclaw CLI — Python 入口

用法:
    ./geekclaw.py <命令> [选项]

命令:
    agent      直接与 Agent 交互
    gateway    启动 Gateway 服务
    auth       管理身份验证 (login / logout / status / models)
    cron       管理定时任务 (list / add / remove / enable / disable)
    skills     管理技能 (list / install / remove / search / show)
    status     显示系统状态
    version    显示版本信息
"""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import uuid
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
    CYAN   = '\033[0;36m'
    BOLD   = '\033[1m'
    NC     = '\033[0m'

    @classmethod
    def disable(cls):
        for attr in ('RED', 'GREEN', 'YELLOW', 'BLUE', 'CYAN', 'BOLD', 'NC'):
            setattr(cls, attr, '')


def _c(text: str, color: str) -> str:
    return f"{color}{text}{Colors.NC}"


# ---------------------------------------------------------------------------
# 环境 / 路径解析
# ---------------------------------------------------------------------------

def get_geekclaw_home() -> Path:
    env = os.environ.get("GEEKCLAW_HOME")
    if env:
        return Path(env)
    return Path(__file__).resolve().parent / "geekclaw-app"


def get_config_path() -> Optional[Path]:
    env = os.environ.get("GEEKCLAW_CONFIG")
    if env:
        return Path(env)
    home = get_geekclaw_home()
    for p in [
        home / "configs" / "config.yaml",
        home / "configs" / "config.json",
        home / "config.yaml",
        home / "config.json",
    ]:
        if p.exists():
            return p
    return home / "configs" / "config.yaml"   # 默认（可能不存在）


def get_app_dir() -> Optional[Path]:
    """返回 geekclaw-app/ 目录（如果存在）"""
    app = Path(__file__).resolve().parent / "geekclaw-app"
    return app if app.exists() else None


def get_go_binary() -> Optional[Path]:
    """按优先级查找编译好的 Go 二进制"""
    project_root = Path(__file__).resolve().parent
    candidates = [
        project_root / "geekclaw-app" / "bin" / "geekclaw-cli",
    ]
    which = shutil.which("geekclaw-cli")
    if which:
        candidates.append(Path(which))
    for p in candidates:
        if p.exists() and os.access(p, os.X_OK):
            return p
    return None


# ---------------------------------------------------------------------------
# 配置加载
# ---------------------------------------------------------------------------

def load_config() -> dict:
    path = get_config_path()
    if not path or not path.exists():
        return {}
    try:
        if path.suffix in (".yaml", ".yml"):
            try:
                import yaml
                with open(path) as f:
                    return yaml.safe_load(f) or {}
            except ImportError:
                # PyYAML 未安装，降级为简单解析
                return _simple_yaml_load(path)
        else:
            return json.loads(path.read_text())
    except Exception:
        return {}


def _simple_yaml_load(path: Path) -> dict:
    """极简 YAML 解析——只取顶层字符串 key"""
    result: dict = {}
    try:
        for line in path.read_text().splitlines():
            m = re.match(r'^(\w[\w_]*):\s*(.+)', line)
            if m:
                result[m.group(1)] = m.group(2).strip()
    except Exception:
        pass
    return result


def config_logs_dir(cfg: dict) -> Path:
    home = get_geekclaw_home()
    return home / "logs"


def config_plugins_dir(cfg: dict) -> Optional[Path]:
    try:
        val = cfg.get("agents", {}).get("defaults", {}).get("plugins_dir", "")
        if val:
            return Path(os.path.expanduser(val))
    except Exception:
        pass
    return None


# ---------------------------------------------------------------------------
# 原子写文件
# ---------------------------------------------------------------------------

def write_atomic(path: Path, data: str):
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(data)
    tmp.replace(path)


# ---------------------------------------------------------------------------
# 委托 Go 二进制执行
# ---------------------------------------------------------------------------

def delegate_to_go(args: list[str], extra_env: Optional[dict] = None):
    """将命令转发给编译好的 Go 二进制，透传所有参数和退出码。"""
    binary = get_go_binary()
    if not binary:
        print(_c("错误: 找不到 geekclaw-cli 二进制，请先运行 './manage.py build'", Colors.RED), file=sys.stderr)
        sys.exit(1)
    # 抑制 Go binary 自身的 banner（geekclaw.py 已经打印过了）
    env = {**os.environ, "GEEKCLAW_NO_BANNER": "1", **(extra_env or {})}
    result = subprocess.run([str(binary)] + args, env=env)
    sys.exit(result.returncode)


# ---------------------------------------------------------------------------
# version 命令
# ---------------------------------------------------------------------------

def cmd_version():
    project_root = Path(__file__).resolve().parent

    # 尝试从 git 获取
    def capture(cmd):
        try:
            return subprocess.check_output(cmd, stderr=subprocess.DEVNULL, text=True).strip()
        except Exception:
            return ""

    version    = capture(["git", "describe", "--tags", "--always", "--dirty"]) or "dev"
    git_commit = capture(["git", "rev-parse", "--short=8", "HEAD"]) or ""

    ver_str = version
    if git_commit:
        ver_str += f" (git: {git_commit})"

    print(f"{_c('geekclaw', Colors.BOLD)} {_c(ver_str, Colors.CYAN)}")

    # 从 go binary 补充 build info（如果有）
    binary = get_go_binary()
    if binary:
        result = subprocess.run([str(binary), "version"],
                                capture_output=True, text=True)
        if result.returncode == 0:
            for line in result.stdout.splitlines():
                if line.strip() and not line.startswith("geekclaw"):
                    print(f"  {line.strip()}")


# ---------------------------------------------------------------------------
# status 命令
# ---------------------------------------------------------------------------

def cmd_status():
    cfg        = load_config()
    home       = get_geekclaw_home()
    cfg_path   = get_config_path()
    logs_dir   = config_logs_dir(cfg)
    plugins    = config_plugins_dir(cfg)
    binary     = get_go_binary()

    print(_c("🦞 geekclaw Status", Colors.BOLD))
    print()

    # 版本
    def capture(cmd):
        try:
            return subprocess.check_output(cmd, stderr=subprocess.DEVNULL, text=True).strip()
        except Exception:
            return "dev"

    version = capture(["git", "describe", "--tags", "--always", "--dirty"])
    print(f"Version:  {version}")
    print(f"Home:     {home}")
    print()

    # 配置
    cfg_ok = cfg_path and cfg_path.exists()
    cfg_mark = _c("✓", Colors.GREEN) if cfg_ok else _c("✗", Colors.RED)
    print(f"Config:   {cfg_path}  {cfg_mark}")

    # Plugins 目录
    if plugins:
        plug_mark = _c("✓", Colors.GREEN) if plugins.exists() else _c("✗", Colors.RED)
        print(f"Plugins:  {plugins}  {plug_mark}")

    # Binary
    bin_mark = _c("✓", Colors.GREEN) if binary else _c("✗", Colors.RED)
    print(f"Binary:   {binary or '(not found)'}  {bin_mark}")
    print()

    # 模型配置
    try:
        model_name = cfg.get("agents", {}).get("defaults", {}).get("model_name", "")
        model_list = cfg.get("model_list", [])
        if model_name:
            print(f"Model:    {model_name}")
        if model_list:
            print(f"Models configured: {len(model_list)}")
            for m in model_list:
                name = m.get("model_name", "")
                model = m.get("model", "")
                base = m.get("api_base", "")
                print(f"  • {_c(name, Colors.CYAN)}  {model}  {_c(base, Colors.BLUE)}")
    except Exception:
        pass
    print()

    # Auth store
    auth_file = logs_dir / "auth.json"
    if auth_file.exists():
        try:
            store = json.loads(auth_file.read_text())
            creds = store.get("credentials", {})
            if creds:
                print(_c("Auth:", Colors.BOLD))
                now = datetime.now(timezone.utc)
                for provider, cred in creds.items():
                    method = cred.get("auth_method", "")
                    email  = cred.get("email", "")
                    exp_str = cred.get("expires_at", "")
                    status = _c("authenticated", Colors.GREEN)
                    if exp_str:
                        try:
                            exp = datetime.fromisoformat(exp_str.replace("Z", "+00:00"))
                            if exp < now:
                                status = _c("expired", Colors.RED)
                        except Exception:
                            pass
                    detail = f"  ({email})" if email else ""
                    print(f"  {_c(provider, Colors.CYAN)} ({method}): {status}{detail}")
                print()
        except Exception:
            pass

    # 进程状态
    gw_running = bool(subprocess.run(
        ["pgrep", "-f", "geekclaw.*gateway"],
        capture_output=True).returncode == 0)
    gw_mark = _c("running", Colors.GREEN) if gw_running else _c("stopped", Colors.YELLOW)
    print(f"Gateway:  {gw_mark}")


# ---------------------------------------------------------------------------
# auth 命令
# ---------------------------------------------------------------------------

def cmd_auth(args):
    # login 委托 Go binary（OAuth / token flow 复杂）
    if args.auth_cmd == "login":
        extra = ["auth", "login", "--provider", args.provider]
        if getattr(args, "device_code", False):
            extra.append("--device-code")
        if getattr(args, "setup_token", False):
            extra.append("--setup-token")
        delegate_to_go(extra)

    elif args.auth_cmd == "logout":
        cfg      = load_config()
        logs_dir = config_logs_dir(cfg)
        auth_file = logs_dir / "auth.json"

        if not auth_file.exists():
            print("未找到凭证文件")
            return

        store = json.loads(auth_file.read_text())
        creds = store.get("credentials", {})

        provider = getattr(args, "provider", "") or ""
        if provider:
            if provider in creds:
                del creds[provider]
                write_atomic(auth_file, json.dumps(store, indent=2))
                print(f"已退出: {provider}")
            else:
                print(f"未找到 '{provider}' 的凭证")
        else:
            auth_file.unlink()
            print("已退出所有提供商")

    elif args.auth_cmd == "status":
        cfg      = load_config()
        logs_dir = config_logs_dir(cfg)
        auth_file = logs_dir / "auth.json"

        if not auth_file.exists():
            print("未登录任何提供商")
            return

        store = json.loads(auth_file.read_text())
        creds = store.get("credentials", {})
        if not creds:
            print("未登录任何提供商")
            return

        now = datetime.now(timezone.utc)
        print(_c("已认证的提供商:", Colors.BOLD))
        for provider, cred in creds.items():
            method    = cred.get("auth_method", "")
            email     = cred.get("email", "")
            project   = cred.get("project_id", "")
            exp_str   = cred.get("expires_at", "")
            status    = _c("active", Colors.GREEN)
            exp_info  = ""
            if exp_str:
                try:
                    exp = datetime.fromisoformat(exp_str.replace("Z", "+00:00"))
                    if exp < now:
                        status = _c("expired", Colors.RED)
                    exp_info = f"  expires: {exp.strftime('%Y-%m-%d %H:%M')}"
                except Exception:
                    pass
            print(f"  {_c(provider, Colors.CYAN)}  method={method}  status={status}")
            if email:   print(f"    email:   {email}")
            if project: print(f"    project: {project}")
            if exp_info: print(f"   {exp_info}")

    elif args.auth_cmd == "models":
        delegate_to_go(["auth", "models"])

    else:
        print("用法: geekclaw.py auth <login|logout|status|models>")
        sys.exit(1)


# ---------------------------------------------------------------------------
# cron 命令
# ---------------------------------------------------------------------------

class CronManager:
    def __init__(self, store_path: Path):
        self.path = store_path

    def _load(self) -> dict:
        if not self.path.exists():
            return {"version": 1, "jobs": []}
        return json.loads(self.path.read_text())

    def _save(self, store: dict):
        write_atomic(self.path, json.dumps(store, indent=2))

    def list_jobs(self) -> list:
        return self._load().get("jobs", [])

    def add_job(self, name: str, message: str,
                every_sec: int = 0, cron_expr: str = "",
                deliver: bool = False, channel: str = "", to: str = "") -> dict:
        now_ms = int(datetime.now(timezone.utc).timestamp() * 1000)

        if every_sec:
            schedule = {"kind": "every", "everyMs": every_sec * 1000}
            next_ms  = now_ms + every_sec * 1000
        elif cron_expr:
            schedule = {"kind": "cron", "expr": cron_expr}
            next_ms  = None   # gateway 启动时计算
        else:
            raise ValueError("必须指定 --every 或 --cron")

        job = {
            "id":      str(uuid.uuid4())[:8],
            "name":    name,
            "enabled": True,
            "schedule": schedule,
            "payload": {
                "kind":    "agent_turn",
                "message": message,
                "deliver": deliver,
                "channel": channel,
                "to":      to,
            },
            "state": {"nextRunAtMs": next_ms} if next_ms else {},
            "createdAtMs": now_ms,
            "updatedAtMs": now_ms,
            "deleteAfterRun": False,
        }

        store = self._load()
        store["jobs"].append(job)
        self._save(store)
        return job

    def remove_job(self, job_id: str) -> bool:
        store = self._load()
        before = len(store["jobs"])
        store["jobs"] = [j for j in store["jobs"] if j["id"] != job_id]
        if len(store["jobs"]) < before:
            self._save(store)
            return True
        return False

    def set_enabled(self, job_id: str, enabled: bool) -> bool:
        store = self._load()
        for job in store["jobs"]:
            if job["id"] == job_id:
                job["enabled"] = enabled
                job["updatedAtMs"] = int(datetime.now(timezone.utc).timestamp() * 1000)
                self._save(store)
                return True
        return False


def _fmt_ms(ms: Optional[int]) -> str:
    if ms is None:
        return "—"
    try:
        return datetime.fromtimestamp(ms / 1000, tz=timezone.utc).strftime("%Y-%m-%d %H:%M")
    except Exception:
        return str(ms)


def _fmt_schedule(s: dict) -> str:
    kind = s.get("kind", "")
    if kind == "every":
        every_ms = s.get("everyMs", 0)
        secs     = every_ms // 1000
        if secs % 3600 == 0:
            return f"every {secs // 3600}h"
        if secs % 60 == 0:
            return f"every {secs // 60}m"
        return f"every {secs}s"
    if kind == "cron":
        return f"cron({s.get('expr', '')})"
    if kind == "at":
        return f"at({_fmt_ms(s.get('atMs'))})"
    return kind


def cmd_cron(args):
    cfg        = load_config()
    store_path = config_logs_dir(cfg) / "jobs.json"
    mgr        = CronManager(store_path)

    sub = args.cron_cmd

    if sub == "list":
        jobs = mgr.list_jobs()
        if not jobs:
            print("暂无定时任务")
            return
        print(f"{'ID':<10} {'名称':<20} {'状态':<8} {'计划':<20} {'下次运行':<20} 消息")
        print("-" * 100)
        for j in jobs:
            jid      = j.get("id", "")
            name     = j.get("name", "")
            enabled  = _c("启用", Colors.GREEN) if j.get("enabled") else _c("禁用", Colors.YELLOW)
            schedule = _fmt_schedule(j.get("schedule", {}))
            next_run = _fmt_ms(j.get("state", {}).get("nextRunAtMs"))
            message  = j.get("payload", {}).get("message", "")[:40]
            print(f"{jid:<10} {name:<20} {enabled:<8} {schedule:<20} {next_run:<20} {message}")

    elif sub == "add":
        try:
            job = mgr.add_job(
                name     = args.name,
                message  = args.message,
                every_sec= args.every or 0,
                cron_expr= args.cron or "",
                deliver  = args.deliver,
                channel  = args.channel or "",
                to       = args.to or "",
            )
            print(_c(f"已添加任务: {job['id']}  {job['name']}", Colors.GREEN))
        except ValueError as e:
            print(_c(f"错误: {e}", Colors.RED), file=sys.stderr)
            sys.exit(1)

    elif sub == "remove":
        if mgr.remove_job(args.job_id):
            print(_c(f"已删除任务: {args.job_id}", Colors.GREEN))
        else:
            print(_c(f"未找到任务: {args.job_id}", Colors.RED), file=sys.stderr)
            sys.exit(1)

    elif sub == "enable":
        if mgr.set_enabled(args.job_id, True):
            print(_c(f"已启用: {args.job_id}", Colors.GREEN))
        else:
            print(_c(f"未找到任务: {args.job_id}", Colors.RED), file=sys.stderr)
            sys.exit(1)

    elif sub == "disable":
        if mgr.set_enabled(args.job_id, False):
            print(_c(f"已禁用: {args.job_id}", Colors.YELLOW))
        else:
            print(_c(f"未找到任务: {args.job_id}", Colors.RED), file=sys.stderr)
            sys.exit(1)

    else:
        print("用法: geekclaw.py cron <list|add|remove|enable|disable>")
        sys.exit(1)


# ---------------------------------------------------------------------------
# skills 命令
# ---------------------------------------------------------------------------

class SkillsManager:
    def __init__(self, plugins_dir: Optional[Path], geekclaw_home: Path):
        self.plugins_skills = (plugins_dir / "skills") if plugins_dir else None
        self.global_skills  = geekclaw_home / "skills"

    def _skill_roots(self) -> list[Path]:
        roots = []
        if self.plugins_skills and self.plugins_skills.exists():
            roots.append(self.plugins_skills)
        if self.global_skills.exists():
            roots.append(self.global_skills)
        return roots

    def list_skills(self) -> list[dict]:
        seen: set = set()
        skills = []
        for root in self._skill_roots():
            source = "plugins" if root == self.plugins_skills else "global"
            for skill_dir in sorted(root.iterdir()):
                if not skill_dir.is_dir():
                    continue
                skill_md = skill_dir / "SKILL.md"
                if not skill_md.exists():
                    continue
                name = skill_dir.name
                if name in seen:
                    continue
                seen.add(name)
                meta = _parse_skill_meta(skill_md, name)
                skills.append({
                    "name":        name,
                    "source":      source,
                    "description": meta.get("description", ""),
                    "path":        str(skill_md),
                })
        return skills

    def load_skill(self, name: str) -> Optional[str]:
        for root in self._skill_roots():
            skill_md = root / name / "SKILL.md"
            if skill_md.exists():
                content = skill_md.read_text()
                # 去掉 frontmatter
                if content.startswith("---"):
                    end = content.find("---", 3)
                    if end != -1:
                        content = content[end + 3:].lstrip()
                return content
        return None

    def install_from_github(self, repo: str):
        """从 GitHub 下载 SKILL.md 并安装"""
        import urllib.request
        # 支持 "owner/repo/skill" 或 "owner/repo"
        parts = repo.strip("/").split("/")
        if len(parts) == 3:
            owner, repo_name, skill_name = parts
            raw_url = f"https://raw.githubusercontent.com/{owner}/{repo_name}/main/{skill_name}/SKILL.md"
        elif len(parts) == 2:
            owner, repo_name = parts
            skill_name = repo_name
            raw_url = f"https://raw.githubusercontent.com/{owner}/{repo_name}/main/SKILL.md"
        else:
            print(_c(f"错误: 无效的 GitHub 路径: {repo}", Colors.RED), file=sys.stderr)
            sys.exit(1)

        target_dir = (self.plugins_skills or self.global_skills) / skill_name
        skill_md   = target_dir / "SKILL.md"
        target_dir.mkdir(parents=True, exist_ok=True)

        print(f"下载: {raw_url}")
        try:
            with urllib.request.urlopen(raw_url, timeout=30) as resp:
                content = resp.read().decode()
        except Exception as e:
            shutil.rmtree(target_dir, ignore_errors=True)
            print(_c(f"下载失败: {e}", Colors.RED), file=sys.stderr)
            sys.exit(1)

        skill_md.write_text(content)
        print(_c(f"已安装技能: {skill_name}  →  {skill_md}", Colors.GREEN))

    def uninstall(self, name: str):
        for root in self._skill_roots():
            skill_dir = root / name
            if skill_dir.exists():
                shutil.rmtree(skill_dir)
                print(_c(f"已删除技能: {name}", Colors.GREEN))
                return
        print(_c(f"未找到技能: {name}", Colors.RED), file=sys.stderr)
        sys.exit(1)

    def search(self, query: str, limit: int = 20) -> list[dict]:
        """搜索 ClawHub 技能注册表"""
        cfg         = load_config()
        hub_cfg     = (cfg.get("tools", {})
                         .get("skills", {})
                         .get("registries", {})
                         .get("clawhub", {}))
        base_url    = hub_cfg.get("base_url", "https://clawhub.ai")
        auth_token  = hub_cfg.get("auth_token", "")
        enabled     = hub_cfg.get("enabled", True)

        if not enabled:
            print(_c("ClawHub 注册表已禁用", Colors.YELLOW))
            return []

        import urllib.request
        import urllib.parse

        params  = urllib.parse.urlencode({"q": query, "limit": limit})
        url     = f"{base_url}/api/v1/search?{params}"
        headers = {"Accept": "application/json"}
        if auth_token:
            headers["Authorization"] = f"Bearer {auth_token}"

        try:
            req  = urllib.request.Request(url, headers=headers)
            with urllib.request.urlopen(req, timeout=30) as resp:
                data = json.loads(resp.read().decode())
            return data.get("results", [])
        except Exception as e:
            print(_c(f"搜索失败: {e}", Colors.RED), file=sys.stderr)
            return []


def _parse_skill_meta(skill_md: Path, fallback_name: str) -> dict:
    """解析 SKILL.md 的 frontmatter 或首段"""
    try:
        content = skill_md.read_text()
    except Exception:
        return {"name": fallback_name, "description": ""}

    name = fallback_name
    description = ""

    if content.startswith("---"):
        end = content.find("---", 3)
        if end != -1:
            fm_text = content[3:end].strip()
            rest    = content[end + 3:].strip()
            # 尝试 YAML / JSON
            try:
                import yaml
                fm = yaml.safe_load(fm_text) or {}
                name        = fm.get("name", fallback_name)
                description = fm.get("description", "")
            except Exception:
                try:
                    fm = json.loads(fm_text)
                    name        = fm.get("name", fallback_name)
                    description = fm.get("description", "")
                except Exception:
                    for line in fm_text.splitlines():
                        m = re.match(r'^(name|description):\s*(.+)', line)
                        if m:
                            if m.group(1) == "name":
                                name = m.group(2).strip()
                            else:
                                description = m.group(2).strip()
            content = rest

    if not description:
        # 从正文提取第一段
        for line in content.splitlines():
            line = line.strip()
            if line and not line.startswith("#"):
                description = line[:200]
                break

    return {"name": name, "description": description}


def cmd_skills(args):
    cfg         = load_config()
    home        = get_geekclaw_home()
    plugins_dir = config_plugins_dir(cfg)
    mgr         = SkillsManager(plugins_dir, home)

    sub = args.skills_cmd

    if sub == "list":
        skills = mgr.list_skills()
        if not skills:
            print("未安装任何技能")
            return
        print(f"{'名称':<24} {'来源':<10} 描述")
        print("-" * 80)
        for s in skills:
            print(f"{_c(s['name'], Colors.CYAN):<34} {s['source']:<10} {s['description'][:48]}")

    elif sub == "show":
        content = mgr.load_skill(args.skill_name)
        if content is None:
            print(_c(f"未找到技能: {args.skill_name}", Colors.RED), file=sys.stderr)
            sys.exit(1)
        print(content)

    elif sub == "install":
        if args.registry:
            # 通过 registry（委托 Go）
            delegate_to_go(["skills", "install", "--registry", args.registry, args.target])
        else:
            mgr.install_from_github(args.target)

    elif sub == "install-builtin":
        delegate_to_go(["skills", "install-builtin"])

    elif sub == "list-builtin":
        delegate_to_go(["skills", "list-builtin"])

    elif sub in ("remove", "rm", "uninstall"):
        mgr.uninstall(args.skill_name)

    elif sub == "search":
        query   = args.query or ""
        results = mgr.search(query)
        if not results:
            print("未找到相关技能")
            return
        print(f"{'名称':<24} {'版本':<10} {'来源':<12} 描述")
        print("-" * 90)
        for r in results:
            name    = r.get("displayName") or r.get("display_name") or r.get("slug", "")
            slug    = r.get("slug", "")
            version = r.get("version", "")
            reg     = r.get("registryName") or r.get("registry_name", "")
            summary = r.get("summary", "")[:48]
            print(f"{_c(name, Colors.CYAN):<34} {version:<10} {reg:<12} {summary}")
            if slug != name:
                print(f"  slug: {slug}")

    else:
        print("用法: geekclaw.py skills <list|show|install|remove|search>")
        sys.exit(1)


# ---------------------------------------------------------------------------
# agent / gateway 命令 (委托 Go)
# ---------------------------------------------------------------------------

def cmd_agent(args):
    check_plugins_env()
    check_model_config()
    extra = ["agent"]
    if getattr(args, "debug", False):
        extra.append("--debug")
    if getattr(args, "message", None):
        extra += ["--message", args.message]
    if getattr(args, "session", None):
        extra += ["--session", args.session]
    if getattr(args, "model", None):
        extra += ["--model", args.model]
    app = get_app_dir()
    env_override = {"GEEKCLAW_HOME": str(app)} if app else {}
    delegate_to_go(extra, extra_env=env_override)


def cmd_gateway(args):
    check_plugins_env()
    check_model_config()
    extra = ["gateway"]
    if getattr(args, "debug", False):
        extra.append("--debug")
    if getattr(args, "no_truncate", False):
        extra.append("--no-truncate")
    # Run from geekclaw-app/ if available, setting GEEKCLAW_HOME accordingly
    app = get_app_dir()
    env_override = {"GEEKCLAW_HOME": str(app)} if app else {}
    delegate_to_go(extra, extra_env=env_override)


# ---------------------------------------------------------------------------
# argparse 树
# ---------------------------------------------------------------------------

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="geekclaw",
        description="geekclaw — Personal AI Assistant",
        add_help=True,
    )
    sub = parser.add_subparsers(dest="command")

    # ── version ──────────────────────────────────────────────────────────────
    sub.add_parser("version", aliases=["v"], help="显示版本信息")

    # ── status ───────────────────────────────────────────────────────────────
    sub.add_parser("status", aliases=["s"], help="显示系统状态")

    # ── agent ────────────────────────────────────────────────────────────────
    p_agent = sub.add_parser("agent", help="直接与 Agent 交互")
    p_agent.add_argument("-d", "--debug",   action="store_true", help="开启 debug 日志")
    p_agent.add_argument("-m", "--message", type=str,            help="单条消息（非交互模式）")
    p_agent.add_argument("-s", "--session", type=str,            help="会话 key")
    p_agent.add_argument("--model",         type=str,            help="使用的模型")

    # ── gateway ───────────────────────────────────────────────────────────────
    p_gw = sub.add_parser("gateway", aliases=["g"], help="启动 Gateway 服务")
    p_gw.add_argument("-d", "--debug",       action="store_true", help="开启 debug 日志")
    p_gw.add_argument("-T", "--no-truncate", action="store_true", dest="no_truncate",
                      help="禁用 debug 日志截断（须与 --debug 同用）")

    # ── auth ─────────────────────────────────────────────────────────────────
    p_auth     = sub.add_parser("auth", help="管理身份验证")
    auth_sub   = p_auth.add_subparsers(dest="auth_cmd")

    p_login    = auth_sub.add_parser("login",  help="登录")
    p_login.add_argument("-p", "--provider",   required=True, help="提供商 (openai / anthropic / google-antigravity)")
    p_login.add_argument("--device-code",      action="store_true", dest="device_code", help="使用设备码流程")
    p_login.add_argument("--setup-token",      action="store_true", dest="setup_token", help="Anthropic setup-token 流程")

    p_logout   = auth_sub.add_parser("logout", help="退出登录")
    p_logout.add_argument("-p", "--provider",  default="", help="提供商（空则退出全部）")

    auth_sub.add_parser("status", help="显示认证状态")
    auth_sub.add_parser("models", help="显示可用模型")

    # ── cron ─────────────────────────────────────────────────────────────────
    p_cron   = sub.add_parser("cron", aliases=["c"], help="管理定时任务")
    cron_sub = p_cron.add_subparsers(dest="cron_cmd")

    cron_sub.add_parser("list", help="列出所有任务")

    p_add = cron_sub.add_parser("add", help="添加定时任务")
    p_add.add_argument("-n", "--name",    required=True, help="任务名称")
    p_add.add_argument("-m", "--message", required=True, help="发送给 Agent 的消息")
    p_add.add_argument("-e", "--every",   type=int,      help="每 N 秒执行一次")
    p_add.add_argument("-c", "--cron",    type=str,      help="Cron 表达式 (如 '0 9 * * *')")
    p_add.add_argument("-d", "--deliver", action="store_true", help="将回复发送到 channel")
    p_add.add_argument("--channel",       type=str,      help="发送目标 channel")
    p_add.add_argument("--to",            type=str,      help="发送目标用户")

    p_rm = cron_sub.add_parser("remove", help="删除任务")
    p_rm.add_argument("job_id", help="任务 ID")

    p_en = cron_sub.add_parser("enable", help="启用任务")
    p_en.add_argument("job_id", help="任务 ID")

    p_dis = cron_sub.add_parser("disable", help="禁用任务")
    p_dis.add_argument("job_id", help="任务 ID")

    # ── skills ────────────────────────────────────────────────────────────────
    p_skills   = sub.add_parser("skills", help="管理技能")
    skills_sub = p_skills.add_subparsers(dest="skills_cmd")

    skills_sub.add_parser("list",          help="列出已安装技能")
    skills_sub.add_parser("list-builtin",  help="列出内置技能")
    skills_sub.add_parser("install-builtin", help="安装所有内置技能")

    p_show = skills_sub.add_parser("show", help="查看技能详情")
    p_show.add_argument("skill_name", help="技能名称")

    p_inst = skills_sub.add_parser("install", help="从 GitHub 或注册表安装技能")
    p_inst.add_argument("target",              help="GitHub 路径 (owner/repo/skill) 或 slug")
    p_inst.add_argument("-r", "--registry",    type=str, default="", help="从指定注册表安装")

    p_rm2 = skills_sub.add_parser("remove", aliases=["rm", "uninstall"], help="删除技能")
    p_rm2.add_argument("skill_name", help="技能名称")

    p_srch = skills_sub.add_parser("search", help="搜索可用技能")
    p_srch.add_argument("query", nargs="?", default="", help="搜索关键词")

    return parser


# ---------------------------------------------------------------------------
# banner
# ---------------------------------------------------------------------------

_B = "\033[1;38;2;62;93;185m"   # blue  — GEEK
_R = "\033[1;38;2;213;70;70m"   # red   — CLAW
_N = "\033[0m"

BANNER = (
    "\n"
    f"{_B} ██████╗ ███████╗███████╗██╗  ██╗{_R} ██████╗██╗      █████╗ ██╗    ██╗\n"
    f"{_B}██╔════╝ ██╔════╝██╔════╝██║ ██╔╝{_R}██╔════╝██║     ██╔══██╗██║    ██║\n"
    f"{_B}██║  ███╗█████╗  █████╗  █████╔╝ {_R}██║     ██║     ███████║██║ █╗ ██║\n"
    f"{_B}██║   ██║██╔══╝  ██╔══╝  ██╔═██╗ {_R}██║     ██║     ██╔══██║██║███╗██║\n"
    f"{_B}╚██████╔╝███████╗███████╗██║  ██╗{_R}╚██████╗███████╗██║  ██║╚███╔███╔╝\n"
    f"{_B} ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝{_R} ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝\n"
    f"{_N}"
)


def print_banner():
    if sys.stdout.isatty():
        print(BANNER)


# ---------------------------------------------------------------------------
# config pre-flight check
# ---------------------------------------------------------------------------

def check_plugins_env():
    """检查 plugins/ 下必要的 Python 运行时包是否存在。"""
    home = get_geekclaw_home()
    plugins_dir = home / "plugins"
    required = ["providers", "sdk", "channels"]
    missing = [p for p in required if not (plugins_dir / p).is_dir()]
    if missing:
        print(_c(f"错误: plugins/ 缺少以下 Python 包: {', '.join(missing)}", Colors.RED), file=sys.stderr)
        print(f"请将对应包复制到: {plugins_dir}/", file=sys.stderr)
        print(f"  例如: cp -r ~/projects/geekclaw-plugins/providers {plugins_dir}/", file=sys.stderr)
        sys.exit(1)


def check_model_config():
    """检查 model_list 是否已配置，未配置则给出友好提示并退出。"""
    cfg = load_config()
    model_list = cfg.get("model_list", [])
    if not model_list:
        cfg_path = get_config_path()
        print(_c("错误: 未配置任何模型", Colors.RED), file=sys.stderr)
        print(f"请编辑配置文件并添加 model_list 条目:", file=sys.stderr)
        print(f"  {cfg_path}", file=sys.stderr)
        print(file=sys.stderr)
        print("示例:", file=sys.stderr)
        print("  model_list:", file=sys.stderr)
        print("    - model_name: my-model", file=sys.stderr)
        print("      model: openai/gpt-4o", file=sys.stderr)
        print("      api_key: sk-xxx", file=sys.stderr)
        print("      api_base: https://openrouter.ai/api/v1", file=sys.stderr)
        sys.exit(1)


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main():
    if not sys.stdout.isatty():
        Colors.disable()

    print_banner()

    parser = build_parser()
    args   = parser.parse_args()

    if args.command is None:
        parser.print_help()
        sys.exit(0)

    cmd = args.command

    if cmd in ("version", "v"):
        cmd_version()
    elif cmd in ("status", "s"):
        cmd_status()
    elif cmd == "agent":
        cmd_agent(args)
    elif cmd in ("gateway", "g"):
        cmd_gateway(args)
    elif cmd == "auth":
        if not getattr(args, "auth_cmd", None):
            parser.parse_args(["auth", "--help"])
        else:
            cmd_auth(args)
    elif cmd in ("cron", "c"):
        if not getattr(args, "cron_cmd", None):
            parser.parse_args(["cron", "--help"])
        else:
            cmd_cron(args)
    elif cmd == "skills":
        if not getattr(args, "skills_cmd", None):
            parser.parse_args(["skills", "--help"])
        else:
            cmd_skills(args)
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
