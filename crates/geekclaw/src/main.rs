//! GeekClaw - 超轻量级个人 AI Agent
//!
//! 最小核心：消息总线 + LLM 调用 + 工具执行 + 会话持久化 + 定时任务

use clap::{Parser, Subcommand};
use std::sync::Arc;

const BANNER: &str = r#"
 ██████╗ ███████╗███████╗██╗  ██╗ ██████╗██╗      █████╗ ██╗    ██╗
██╔════╝ ██╔════╝██╔════╝██║ ██╔╝██╔════╝██║     ██╔══██╗██║    ██║
██║  ███╗█████╗  █████╗  █████╔╝ ██║     ██║     ███████║██║ █╗ ██║
██║   ██║██╔══╝  ██╔══╝  ██╔═██╗ ██║     ██║     ██╔══██║██║███╗██║
╚██████╔╝███████╗███████╗██║  ██╗╚██████╗███████╗██║  ██║╚███╔███╔╝
 ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝ ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝
"#;

const VERSION: &str = env!("CARGO_PKG_VERSION");

#[derive(Parser)]
#[command(name = "geekclaw", about = "GeekClaw - Personal AI Agent")]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,

    /// 配置文件路径
    #[arg(short, long, default_value = "config.yaml")]
    config: String,
}

#[derive(Subcommand)]
enum Commands {
    /// 启动 Agent（交互模式）
    Agent,
    /// 显示版本信息
    Version,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Banner
    if std::env::var("GEEKCLAW_NO_BANNER").is_err() {
        eprintln!("{BANNER}");
    }

    // 初始化日志
    geekclaw_logger::init();

    let cli = Cli::parse();

    match cli.command {
        Some(Commands::Version) | None => {
            println!("geekclaw v{VERSION}");
            if cli.command.is_none() {
                println!("Use --help for usage information.");
            }
        }
        Some(Commands::Agent) => {
            run_agent(&cli.config).await?;
        }
    }

    Ok(())
}

/// 根据配置创建 LLM Provider。
fn create_provider(
    cfg: &geekclaw_config::Config,
) -> anyhow::Result<Arc<dyn geekclaw_providers::LlmProvider>> {
    // 从 providers 配置中查找第一个可用的 provider。
    if let Some(provider_cfg) = cfg.providers.first() {
        let api_key = provider_cfg
            .api_key
            .as_deref()
            .unwrap_or_default();
        let base_url = provider_cfg
            .base_url
            .as_deref()
            .unwrap_or("https://api.openai.com/v1");
        let model = provider_cfg
            .model
            .as_deref()
            .unwrap_or(&cfg.agents.defaults.model);

        tracing::info!(
            provider = %provider_cfg.name,
            base_url = %base_url,
            model = %model,
            "创建 LLM Provider"
        );

        let provider = geekclaw_providers::OpenAICompatProvider::new(base_url, api_key, model);
        return Ok(Arc::new(provider));
    }

    // 没有配置 provider，尝试从环境变量创建。
    let api_key = std::env::var("OPENAI_API_KEY")
        .or_else(|_| std::env::var("ANTHROPIC_API_KEY"))
        .or_else(|_| std::env::var("DEEPSEEK_API_KEY"))
        .unwrap_or_default();

    let base_url = std::env::var("GEEKCLAW_API_BASE")
        .unwrap_or_else(|_| "https://api.openai.com/v1".to_string());

    if api_key.is_empty() {
        anyhow::bail!(
            "未找到 LLM Provider 配置。请在 config.yaml 中配置 providers，\
             或设置 OPENAI_API_KEY / ANTHROPIC_API_KEY 环境变量"
        );
    }

    let model = &cfg.agents.defaults.model;
    tracing::info!(
        base_url = %base_url,
        model = %model,
        "从环境变量创建 LLM Provider"
    );

    let provider = geekclaw_providers::OpenAICompatProvider::new(&base_url, &api_key, model);
    Ok(Arc::new(provider))
}

/// 启动 Agent 主循环（交互式 stdin 模式）。
async fn run_agent(config_path: &str) -> anyhow::Result<()> {
    use std::path::Path;
    use tokio::io::{AsyncBufReadExt, BufReader};

    // 加载配置。
    let cfg = if Path::new(config_path).exists() {
        geekclaw_config::load(config_path)?
    } else {
        tracing::warn!("配置文件 {config_path} 不存在，使用默认配置");
        geekclaw_config::load_default()
    };

    // 创建 LLM Provider。
    let provider = create_provider(&cfg)?;

    // 创建消息总线。
    let bus = geekclaw_bus::MessageBus::new();
    let outbound_tx = bus.outbound_sender();

    // 创建会话存储。
    let data_dir = std::env::var("GEEKCLAW_DATA_DIR")
        .unwrap_or_else(|_| ".geekclaw/data".to_string());
    let memory: Arc<dyn geekclaw_memory::SessionStore> = Arc::new(
        geekclaw_memory::JSONLStore::new(
            format!("{data_dir}/sessions"),
            format!("{data_dir}/meta"),
        )
        .await?,
    );

    // 创建工具注册表并注册内置工具。
    let tools = Arc::new(geekclaw_tools::ToolRegistry::new());
    tools.register(Arc::new(geekclaw_tools::builtin::ShellTool));
    tools.register(Arc::new(geekclaw_tools::builtin::FileSystemTool));
    tracing::info!(count = tools.len(), "内置工具注册完成");

    // 创建 Agent 实例。
    let agent_instance = geekclaw_agent::AgentInstance::from_config(&cfg);

    // 创建 Agent 循环。
    let agent = geekclaw_agent::AgentLoop::new(
        agent_instance,
        outbound_tx.clone(),
        memory,
        provider,
        tools,
    );

    // 启动出站消息打印任务���
    let mut outbound_bus = bus;
    let print_task = tokio::spawn(async move {
        while let Some(msg) = outbound_bus.consume_outbound().await {
            println!("\n{}\n", msg.content);
            print!("You: ");
            // flush stdout
            use std::io::Write;
            let _ = std::io::stdout().flush();
        }
    });

    // 交互式 stdin 循环。
    eprintln!("GeekClaw v{VERSION} | 输入消息开始对话，Ctrl+C 退出\n");
    print!("You: ");
    use std::io::Write;
    let _ = std::io::stdout().flush();

    let stdin = BufReader::new(tokio::io::stdin());
    let mut lines = stdin.lines();
    let session_key = "interactive:default".to_string();

    while let Ok(Some(line)) = lines.next_line().await {
        let line = line.trim().to_string();
        if line.is_empty() {
            print!("You: ");
            let _ = std::io::stdout().flush();
            continue;
        }

        if line == "/quit" || line == "/exit" {
            break;
        }

        let opts = geekclaw_agent::ProcessOptions {
            session_key: session_key.clone(),
            channel: "interactive".into(),
            chat_id: "stdin".into(),
            user_message: line,
            send_response: true,
            ..Default::default()
        };

        match agent.process_message(opts).await {
            Ok(response) => {
                if !response.is_empty() {
                    println!("\nAssistant: {response}\n");
                }
                print!("You: ");
                let _ = std::io::stdout().flush();
            }
            Err(e) => {
                eprintln!("\n错误: {e}\n");
                print!("You: ");
                let _ = std::io::stdout().flush();
            }
        }
    }

    agent.stop();
    print_task.abort();
    eprintln!("\nBye!");

    Ok(())
}
