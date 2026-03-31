package status

import (
	"fmt"
	"os"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// statusCmd 加载配置并显示版本、配置路径、模型和提供商的状态信息。
func statusCmd() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	configPath := internal.GetConfigPath()

	fmt.Printf("%s geekclaw Status\n", internal.Logo)
	fmt.Printf("Version: %s\n", config.FormatVersion())
	build, _ := config.FormatBuildInfo()
	if build != "" {
		fmt.Printf("Build: %s\n", build)
	}
	fmt.Println()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("Config:", configPath, "✓")
	} else {
		fmt.Println("Config:", configPath, "✗")
	}

	pluginsDir := cfg.PluginsPath()
	if _, err := os.Stat(pluginsDir); err == nil {
		fmt.Println("Plugins directory:", pluginsDir, "✓")
	} else {
		fmt.Println("Plugins directory:", pluginsDir, "✗")
	}

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Model: %s\n", cfg.Agents.Defaults.GetModelName())

		hasOpenRouter := cfg.Providers.OpenRouter.APIKey != ""
		hasAnthropic := cfg.Providers.Anthropic.APIKey != ""
		hasOpenAI := cfg.Providers.OpenAI.APIKey != ""
		hasGemini := cfg.Providers.Gemini.APIKey != ""
		hasZhipu := cfg.Providers.Zhipu.APIKey != ""
		hasQwen := cfg.Providers.Qwen.APIKey != ""
		hasGroq := cfg.Providers.Groq.APIKey != ""
		hasVLLM := cfg.Providers.VLLM.APIBase != ""
		hasMoonshot := cfg.Providers.Moonshot.APIKey != ""
		hasDeepSeek := cfg.Providers.DeepSeek.APIKey != ""
		hasVolcEngine := cfg.Providers.VolcEngine.APIKey != ""
		hasNvidia := cfg.Providers.Nvidia.APIKey != ""
		hasOllama := cfg.Providers.Ollama.APIBase != ""

		status := func(enabled bool) string {
			if enabled {
				return "✓"
			}
			return "not set"
		}
		fmt.Println("OpenRouter API:", status(hasOpenRouter))
		fmt.Println("Anthropic API:", status(hasAnthropic))
		fmt.Println("OpenAI API:", status(hasOpenAI))
		fmt.Println("Gemini API:", status(hasGemini))
		fmt.Println("Zhipu API:", status(hasZhipu))
		fmt.Println("Qwen API:", status(hasQwen))
		fmt.Println("Groq API:", status(hasGroq))
		fmt.Println("Moonshot API:", status(hasMoonshot))
		fmt.Println("DeepSeek API:", status(hasDeepSeek))
		fmt.Println("VolcEngine API:", status(hasVolcEngine))
		fmt.Println("Nvidia API:", status(hasNvidia))
		if hasVLLM {
			fmt.Printf("vLLM/Local: ✓ %s\n", cfg.Providers.VLLM.APIBase)
		} else {
			fmt.Println("vLLM/Local: not set")
		}
		if hasOllama {
			fmt.Printf("Ollama: ✓ %s\n", cfg.Providers.Ollama.APIBase)
		} else {
			fmt.Println("Ollama: not set")
		}

	}
}
