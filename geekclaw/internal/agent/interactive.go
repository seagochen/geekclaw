package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/agent"
)

// 交互模式标识符
const (
	modePico   = "pico"   // 聊天模式（默认）- 输入发送给 AI 代理
	modeCmd    = "cmd"    // 命令模式 - 输入作为 shell 命令执行
	modeHiPico = "hipico" // HiPico 模式 - 命令模式中的 AI 辅助
)

// interactiveMode 启动基于 readline 的交互式会话，支持聊天模式和命令模式之间切换。
func interactiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	chatPrompt := fmt.Sprintf("%s You: ", internal.Logo)
	cmdPrompt := "$ "

	mode := modePico

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          chatPrompt,
		HistoryFile:     filepath.Join(os.TempDir(), ".geekclaw_history"),
		HistoryLimit:    100,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("Error initializing readline: %v\n", err)
		fmt.Println("Falling back to simple input mode...")
		simpleInteractiveMode(agentLoop, sessionKey)
		return
	}
	defer rl.Close()

	hipicoSessionKey := "cli:hipico"

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		// :help 和 :usage 在所有模式下均可使用
		if input == ":help" {
			printInteractiveHelp()
			continue
		}
		if input == ":usage" {
			printUsage(agentLoop)
			continue
		}

		switch mode {
		case modePico:
			if input == ":cmd" {
				mode = modeCmd
				rl.SetPrompt(cmdPrompt)
				fmt.Println("Switched to command mode. Type :pico to return to chat.")
				continue
			}

			ctx := context.Background()
			response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			fmt.Printf("\n%s %s\n\n", internal.Logo, response)

		case modeCmd:
			if input == ":pico" {
				mode = modePico
				rl.SetPrompt(chatPrompt)
				fmt.Println("Switched to chat mode. Type :cmd to return to command mode.")
				continue
			}

			if strings.HasPrefix(input, ":hipico") {
				initialMsg := strings.TrimSpace(strings.TrimPrefix(input, ":hipico"))
				if initialMsg == "" {
					fmt.Println("Usage: :hipico <message>")
					fmt.Println("Example: :hipico check the log files for error messages")
					continue
				}

				ctx := context.Background()
				response, err := agentLoop.ProcessDirectWithWorkDir(ctx, initialMsg, hipicoSessionKey, cmdWorkingDir)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}
				fmt.Printf("%s %s\n\n", internal.Logo, response)
				continue
			}

			executeShellCommand(input)

		case modeHiPico:
			if input == "/byepico" {
				mode = modeCmd
				rl.SetPrompt(cmdPrompt)
				fmt.Println("AI assistance ended. Back to command mode.")
				continue
			}

			if input == "/pico" {
				mode = modePico
				rl.SetPrompt(chatPrompt)
				fmt.Println("AI assistance ended. Switched to chat mode.")
				continue
			}

			ctx := context.Background()
			response, err := agentLoop.ProcessDirect(ctx, input, hipicoSessionKey)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			fmt.Printf("\n%s %s\n\n", internal.Logo, response)
		}
	}
}

// simpleInteractiveMode 在 readline 不可用时提供简单的交互模式作为回退方案。
func simpleInteractiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	reader := bufio.NewReader(os.Stdin)
	mode := modePico
	hipicoSessionKey := "cli:hipico"

	for {
		switch mode {
		case modePico:
			fmt.Printf("%s You: ", internal.Logo)
		case modeCmd:
			fmt.Print("$ ")
		case modeHiPico:
			fmt.Printf("%s> ", internal.Logo)
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		// :help 和 :usage 在所有模式下均可使用
		if input == ":help" {
			printInteractiveHelp()
			continue
		}
		if input == ":usage" {
			printUsage(agentLoop)
			continue
		}

		switch mode {
		case modePico:
			if input == ":cmd" {
				mode = modeCmd
				fmt.Println("Switched to command mode. Type :pico to return to chat.")
				continue
			}

			ctx := context.Background()
			response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			fmt.Printf("\n%s %s\n\n", internal.Logo, response)

		case modeCmd:
			if input == ":pico" {
				mode = modePico
				fmt.Println("Switched to chat mode. Type :cmd to return to command mode.")
				continue
			}

			if strings.HasPrefix(input, ":hipico") {
				initialMsg := strings.TrimSpace(strings.TrimPrefix(input, ":hipico"))
				if initialMsg == "" {
					fmt.Println("Usage: :hipico <message>")
					fmt.Println("Example: :hipico check the log files for error messages")
					continue
				}

				ctx := context.Background()
				response, err := agentLoop.ProcessDirectWithWorkDir(ctx, initialMsg, hipicoSessionKey, cmdWorkingDir)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}
				fmt.Printf("%s %s\n\n", internal.Logo, response)
				continue
			}

			executeShellCommand(input)

		case modeHiPico:
			if input == "/byepico" {
				mode = modeCmd
				fmt.Println("AI assistance ended. Back to command mode.")
				continue
			}

			if input == "/pico" {
				mode = modePico
				fmt.Println("AI assistance ended. Switched to chat mode.")
				continue
			}

			ctx := context.Background()
			response, err := agentLoop.ProcessDirect(ctx, input, hipicoSessionKey)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			fmt.Printf("\n%s %s\n\n", internal.Logo, response)
		}
	}
}

