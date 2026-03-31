package agent

import (
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/agent"
)

// printInteractiveHelp 输出所有交互模式的详细使用说明。
func printInteractiveHelp() {
	fmt.Printf(`%s GeekClaw Interactive Mode Help
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

GeekClaw has two interactive modes:

  1. Chat Mode (default)
     Talk to the AI agent directly. Your input is sent as a message
     and the AI responds.

  2. Command Mode
     Execute shell commands directly, like a terminal. Supports cd,
     pipes, redirects, and all standard shell features.
     Use :hipico to ask AI for one-shot help within command mode.

Commands (available in all modes):
  :help      Show this help message
  :usage     Show model info and token usage
  exit       Exit GeekClaw
  quit       Exit GeekClaw
  Ctrl+C     Exit GeekClaw

Mode switching:
  :cmd           Switch to command mode     (from chat mode)
  :pico          Switch to chat mode        (from command mode)
  :hipico <msg>  Ask AI for help            (from command mode, one-shot)

Examples:
  Chat mode:
    %s You: What is the weather today?

  Command mode:
    $ ls -al /var/log
    $ cd /tmp
    $ cat error.log | grep "FATAL"

  AI help (one-shot, from command mode):
    $ :hipico check the log files for errors
    $ :hipico what does this error mean in syslog

`, internal.Logo, internal.Logo)
}

// printUsage 显示当前模型信息和累计的 token 使用量。
func printUsage(agentLoop *agent.AgentLoop) {
	info := agentLoop.GetUsageInfo()
	if info == nil {
		fmt.Println("No usage information available.")
		return
	}
	fmt.Printf(`%s Usage
━━━━━━━━━━━━━━━━━━━━━━
Model:              %s
Max tokens:         %d
Temperature:        %.1f

Token usage (this session):
  Prompt tokens:    %d
  Completion tokens:%d
  Total tokens:     %d
  Requests:         %d
`, internal.Logo,
		info["model"],
		info["max_tokens"],
		info["temperature"],
		info["prompt_tokens"],
		info["completion_tokens"],
		info["total_tokens"],
		info["requests"],
	)
}
