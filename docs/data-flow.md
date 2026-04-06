# 业务逻辑与数据流

## 主程序启动流程

```mermaid
flowchart TD
    Start([geekclaw gateway]) --> LoadCfg[加载配置文件\nconfig.LoadConfig]
    LoadCfg --> CreateProvider[创建 LLM Provider\nproviders.CreateProvider]
    CreateProvider --> NewBus[创建消息总线\nbus.NewMessageBus]
    NewBus --> NewAgent[创建 AgentLoop\nagent.NewAgentLoop]

    NewAgent --> RegAgents[注册所有 AgentInstance\nAgentRegistry]
    RegAgents --> RegTools[为每个 Agent 注册工具\nToolRegistry]
    RegTools --> LoadPlugins[加载外部插件\n搜索/工具/命令插件]

    LoadPlugins --> SetupCron[初始化 CronService]
    SetupCron --> SetupHB[初始化 HeartbeatService]
    SetupHB --> SetupMedia[初始化 MediaStore\n含 TTL 清理协程]
    SetupMedia --> InitChannels[初始化渠道 Manager\n启动外部渠道子进程]
    InitChannels --> SetupHTTP[注册 HTTP Webhook + 健康检查端点]
    SetupHTTP --> StartAll[channelManager.StartAll\n启动所有渠道 goroutine]
    StartAll --> StartLoop[go agentLoop.Run\n启动 Agent 消息循环]
    StartLoop --> WaitSig{等待 SIGINT}
    WaitSig -->|收到信号| Shutdown[优雅关闭]
    Shutdown --> StopCh[1. cancel context]
    StopCh --> StopChannels[2. channelManager.StopAll\n停止消息源]
    StopChannels --> StopAgent[3. agentLoop.Stop\n停止消费者]
    StopAgent --> StopCron[4. cronService.Stop\n停止定时任务]
    StopCron --> CloseBus[5. msgBus.Close\n最后关闭总线]
    CloseBus --> Cleanup[6. 清理 Provider/Voice/Media]
    Cleanup --> End([退出])
```

---

## 核心业务流程

### 流程1：入站消息处理（渠道 → LLM → 回复）

**触发条件**: 用户通过 Telegram/Discord/CLI 等渠道发送消息
**涉及模块**: Channel → Bus → AgentLoop → Agent → LLM → Channel

```mermaid
sequenceDiagram
    participant U as 用户
    participant CH as Channel 实现
    participant BC as BaseChannel
    participant BUS as MessageBus
    participant AL as AgentLoop
    participant AR as AgentRegistry
    participant AI as AgentInstance
    participant CB as ContextBuilder
    participant LLM as LLMProvider
    participant TOOL as ToolRegistry

    U->>CH: 发送消息（webhook/polling）
    CH->>BC: HandleMessage(peer, senderID, chatID, content, media)
    BC->>BC: IsAllowed(senderID) 鉴权检查
    BC-->>CH: StartTyping / ReactToMessage / SendPlaceholder（可选）
    BC->>BUS: PublishInbound(InboundMessage)

    BUS->>AL: ConsumeInbound() → InboundMessage
    AL->>AL: 语音转录（如有音频）
    AL->>AR: ResolveRoute(channel, peer) → AgentInstance
    AL->>AL: handleCommand? 斜杠命令检查
    AL->>AI: runAgentLoop(ctx, agent, processOptions)

    AI->>AI: LoadHistory + LoadSummary (SessionStore)
    AI->>CB: BuildMessages(history, summary, userMsg, media)
    CB->>CB: BuildSystemPromptWithCache() 读取技能/记忆
    CB-->>AI: []Message（含 system + history + user）

    AI->>AI: resolveMediaRefs → base64 内联
    AI->>AI: AddMessage("user", content)

    loop LLM + 工具循环（最多 MaxIterations 次）
        AI->>LLM: Chat(messages, toolDefs, model, opts)
        LLM-->>AI: LLMResponse{Content, ToolCalls}

        alt 无工具调用
            AI-->>AL: 返回 finalContent，退出循环
        else 有工具调用
            AI->>TOOL: 并行执行所有 ToolCall
            TOOL-->>BUS: 实时 ForUser 反馈（PublishOutbound）
            BUS-->>CH: 分发 OutboundMessage
            CH-->>U: 工具中间结果
            TOOL-->>AI: ToolResult.ForLLM → 追加到 messages
        end
    end

    AI->>AI: AddMessage("assistant", finalContent)
    AI->>AI: Save + maybeSummarize（超阈值异步压缩）

    AL->>BUS: PublishOutbound(finalContent)
    BUS->>CH: 分发 OutboundMessage
    CH->>U: 发送最终回复
```

---

### 流程2：工具执行（同步 vs 异步）

**触发条件**: LLM 返回包含 `ToolCalls` 的响应
**涉及模块**: AgentInstance → ToolRegistry → Tool → Bus

```mermaid
flowchart TD
    LLM_RESP[LLM 返回 ToolCalls] --> NORMALIZE[规范化 ToolCalls 列表]
    NORMALIZE --> PARALLEL{并行执行所有工具}

    PARALLEL --> SYNC[同步工具\nExecTool / WebSearchTool\nMCPTool / MessageTool ...]
    PARALLEL --> ASYNC[异步工具\nSpawnTool\nSubagentTool]

    SYNC --> CTX[注入 ToolContext\nchannel + chatID]
    CTX --> EXEC[tool.Execute ctx args]
    EXEC --> RESULT[ToolResult]

    ASYNC --> ASYNC_EXEC[tool.ExecuteAsync ctx args callback]
    ASYNC_EXEC --> ASYNC_IMM[立即返回 AsyncResult\n告知用户任务已启动]
    ASYNC_EXEC --> BG[后台 goroutine 执行]
    BG --> BG_DONE[执行完成]
    BG_DONE --> CB[asyncCallback result]
    CB --> PUB_OUT[PublishOutbound ForUser\n实时反馈用户]
    CB --> PUB_IN[PublishInbound system 渠道\n触发 Agent 后续处理]

    RESULT --> USER_FEED{ForUser 非空且非 Silent}
    USER_FEED -->|是| OUTBOUND[PublishOutbound → 渠道 → 用户]
    USER_FEED -->|否| SKIP[跳过]

    RESULT --> LLM_FEED[ForLLM → 追加 role:tool 消息]
    LLM_FEED --> NEXT_ITER[下一次 LLM 迭代]

    ASYNC_IMM --> LLM_FEED
```

---

### 流程3：故障转移与模型路由

**触发条件**: LLM 调用失败或启用复杂度路由
**涉及模块**: AgentInstance → Router → FallbackChain → Provider

```mermaid
flowchart TD
    CALL[callLLM request] --> ROUTE{Router 是否启用}
    ROUTE -->|是| CLASSIFY[Router.SelectModel\n分析消息复杂度]
    CLASSIFY --> SCORE{复杂度得分}
    SCORE -->|低于阈值| LIGHT[使用 LightModel\n成本更低的模型]
    SCORE -->|高于阈值| PRIMARY[使用 PrimaryModel]
    ROUTE -->|否| PRIMARY

    PRIMARY --> CANDIDATES[获取 Candidates 列表\n主模型 + Fallbacks]
    LIGHT --> LIGHT_CANDS[获取 LightCandidates 列表]

    CANDIDATES --> FALLBACK[FallbackChain.Execute]
    LIGHT_CANDS --> FALLBACK

    FALLBACK --> CHECK[检查 CooldownTracker\n是否在冷却期]
    CHECK -->|冷却中| NEXT[跳到下一候选]
    CHECK -->|正常| TRY[调用 Provider.Chat]
    TRY --> ERR{出错类型}
    ERR -->|速率限制 429| COOLDOWN[记录冷却 + 转移]
    ERR -->|服务过载 503| COOLDOWN
    ERR -->|超时| RETRY{重试次数}
    RETRY -->|< 2| TRY
    RETRY -->|≥ 2| NEXT
    ERR -->|成功| DONE[返回 LLMResponse]
    COOLDOWN --> NEXT
    NEXT --> EXHAUSTED{还有候选}
    EXHAUSTED -->|是| CHECK
    EXHAUSTED -->|否| FAIL[FallbackExhaustedError]
```

---

### 流程4：系统提示词构建（带 KV Cache 优化）

**触发条件**: 每次 LLM 调用前
**涉及模块**: ContextBuilder → SkillsLoader → SessionStore

```mermaid
flowchart TD
    BUILD[BuildMessages调用] --> STATIC{静态部分缓存有效?}
    STATIC -->|是| USE_CACHE[直接使用缓存提示词\n基于 mtime 检查技能文件]
    STATIC -->|否| REBUILD[重新构建系统提示词]

    REBUILD --> PERSONA[读取 PERSONA.md\n人格设定]
    PERSONA --> SKILLS[buildSkillsSection\n扫描技能文件]
    SKILLS --> MEMORY[读取 MEMORY.md\n记忆文件]
    MEMORY --> STATIC_PROMPT[静态系统提示词]
    STATIC_PROMPT --> CACHE[缓存到 cachedSystemPrompt]

    USE_CACHE --> DYNAMIC[buildDynamicContext\n动态上下文]
    CACHE --> DYNAMIC

    DYNAMIC --> TIME[当前时间]
    DYNAMIC --> RUNTIME[运行时信息\nchannel + chatID]
    DYNAMIC --> SESSION[会话摘要]

    TIME --> COMBINE[组合为 system Message]
    RUNTIME --> COMBINE
    SESSION --> COMBINE

    COMBINE --> CONTENT_BLOCKS[拆分为 ContentBlocks\n支持 Anthropic KV Cache]
    CONTENT_BLOCKS --> MESSAGES[最终 messages 列表\nsystem + history + user]
```

---

### 流程5：异步子 Agent（Spawn Tool）

**触发条件**: 主 Agent 调用 `spawn` 工具
**涉及模块**: SpawnTool → SubagentManager → RunToolLoop → system 渠道 → AgentLoop

```mermaid
sequenceDiagram
    participant MA as 主 AgentLoop
    participant ST as SpawnTool
    participant SM as SubagentManager
    participant BG as 后台 goroutine
    participant RL as RunToolLoop
    participant BUS as MessageBus
    participant MA2 as 主 AgentLoop\n(system 渠道)

    MA->>ST: Execute{task, label}
    ST->>SM: Spawn(ctx, task, label, originChannel, originChatID, callback)
    SM-->>ST: taskID
    ST-->>MA: AsyncResult("已启动子任务 xyz")

    SM->>BG: go runTask(ctx, task, callback)
    Note over BG: 后台运行，不阻塞主循环

    BG->>RL: RunToolLoop(ctx, ToolLoopConfig, messages, ...)
    RL->>RL: LLM + 工具循环
    RL-->>BG: ToolLoopResult{Content, Iterations}

    BG->>BG: callback(ctx, ToolResult)
    BG->>BUS: PublishOutbound(ForUser) 实时通知用户
    BG->>BUS: PublishInbound(channel="system", content=result)

    BUS->>MA2: ConsumeInbound → system 消息
    MA2->>MA2: processSystemMessage(originChannel, originChatID)
    MA2->>BUS: PublishOutbound(子任务完成报告)
```

---

### 流程6：心跳检查

**触发条件**: 定时器触发（最小间隔 5 分钟）
**涉及模块**: HeartbeatService → AgentLoop → LLM

```mermaid
flowchart TD
    TIMER[定时器触发] --> READ[读取 HEARTBEAT.md\n获取心跳提示词]
    READ --> STATE[state.Manager\n查找最后活跃 channel/chatID]
    STATE --> FOUND{找到活跃渠道}
    FOUND -->|是| HANDLER[调用 handler\nrunAgentLoop with HeartbeatPrompt]
    FOUND -->|否| SKIP[跳过本次心跳]
    HANDLER --> LLM[正常 LLM + 工具流程]
    LLM --> REPLY[回复到最后活跃渠道]
```

---

## 关键数据结构流转

```mermaid
flowchart LR
    RAW["平台原始消息\n(JSON/text)"]
    IB["InboundMessage\n{Channel, SenderID, ChatID,\nContent, Media, Peer}"]
    MSG["providers.Message[]\n{Role, Content, ToolCalls,\nSystemParts, Media}"]
    RESP["LLMResponse\n{Content, ToolCalls,\nReasoning, Usage}"]
    TR["ToolResult\n{ForLLM, ForUser,\nIsError, Silent, Media}"]
    OB["OutboundMessage\n{Channel, ChatID, Content,\nMedia}"]

    RAW -->|Channel.HandleMessage| IB
    IB -->|AgentLoop 解析| MSG
    MSG -->|LLMProvider.Chat| RESP
    RESP -->|ToolRegistry.Execute| TR
    TR -->|ForLLM 追加到| MSG
    TR -->|ForUser 发布为| OB
    RESP -->|最终 Content 发布为| OB
    OB -->|Channel.Send| RAW2["平台消息\n(text/media)"]
```
