// Copy for the Jack home page (src/jack/views/JackHomeView.vue).
//
// Kept beside the view as component-local i18n messages, so the upstream locale
// files stay untouched and upstream syncs cannot conflict with them. Keys the
// upstream home page already has (home.login, nav.modelPlaza, ...) are read from
// the global messages instead. Traditional Chinese is written by hand here: the
// generated zh-Hant locale only converts the upstream `zh` tree.

const en = {
  jackHome: {
    eyebrow: 'One key · Every model',
    titleLead: 'One key,',
    titleTail: 'every model.',
    lead: 'Claude, GPT and Gemini behind one gateway. Switch tools without switching keys, switch models without touching code — just point your Base URL here.',
    ctaRegister: 'Create a free account',
    ctaLogin: 'Sign in',
    ctaDashboard: 'Open dashboard',
    ctaGuide: 'Connect in 3 minutes',
    protocolsLabel: 'Speaks',
    diagram: {
      label: 'One key routed through the gateway to several model providers',
      yourKey: 'YOUR KEY',
      gateway: 'Gateway',
      more: '+{n} more',
      models: '{n} models',
      hint: 'Hover a provider to trace its route'
    },
    steps: {
      eyebrow: 'Three strokes',
      title: 'Three strokes, then sign.',
      lead: 'No SDK, no agent to install. Call models exactly the way you already do.',
      s1: { title: 'Create an account', desc: 'Sign up and open the dashboard: subscription, balance and usage on one page.' },
      s2: { title: 'Create a key', desc: 'On the API Keys page, create an sk- key and pick a group.' },
      s3: { title: 'Point it here', desc: 'Set your tool’s Base URL to this site. Everything else stays the same.' }
    },
    code: {
      eyebrow: 'Drop-in',
      title1: 'The tools you know,',
      title2: 'nothing new to learn.',
      check1: 'Claude Code and Codex CLI work as soon as they point here',
      check2: 'Any OpenAI- or Anthropic-compatible SDK connects',
      check3: 'Streaming, tool calls and long context behave as usual',
      baseUrl: 'Your Base URL',
      copy: 'Copy',
      copied: 'Copied',
      keyPlaceholder: 'sk-your-key',
      createKey: 'Create a key'
    },
    models: {
      eyebrow: 'Live catalogue',
      title: 'Models available right now.',
      lead: 'Read straight from the Model Plaza, so this list follows the site’s configuration.',
      count: '{models} models · {platforms} platforms',
      all: 'All',
      more: '+{n} more',
      viewAll: 'See full pricing'
    },
    features: {
      eyebrow: 'Why here',
      title: 'Quiet, reliable, well kept books.',
      lead: 'Good infrastructure stays out of the way. It is always there, and it remembers every request.',
      gateway: {
        eyebrow: 'Unified gateway',
        title: 'One endpoint, several protocols',
        desc: 'Anthropic Messages, OpenAI Chat Completions and Responses side by side. The gateway answers in whatever protocol your client speaks.'
      },
      metered: { eyebrow: 'Metered', title: 'Real-time billing', desc: 'Priced by actual tokens; every request is booked as it happens.' },
      sticky: { eyebrow: 'Sticky session', title: 'Sticky sessions', desc: 'A conversation stays on one upstream, for better cache hits and consistent replies.' },
      resilient: { eyebrow: 'Resilient', title: 'Account pool scheduling', desc: 'Upstream accounts are scheduled automatically and fail over quietly. Your requests never notice.' },
      transparent: { eyebrow: 'Transparent', title: 'Usage in the open', desc: 'Spend per key, per model and per day, all in the dashboard. No black boxes.', figure: 'traceable' }
    },
    faq: {
      eyebrow: 'FAQ',
      title: 'Questions',
      lead: 'Not here? Sign in for the docs, or contact the administrator.',
      q1: 'Do I need to change my code?',
      a1: 'No. Replace the Base URL with this site and the API key with an sk- key from the dashboard. Nothing else changes.',
      q2: 'Which clients are supported?',
      a2: 'Claude Code, Codex CLI, and any SDK, editor plugin or chat front end that supports an OpenAI- or Anthropic-compatible endpoint.',
      q3: 'How is usage billed?',
      a3: 'By actual token usage, deducted in real time, or from your subscription quota. Every request’s model, tokens and cost are listed under Usage.',
      q4: 'What if a key leaks?',
      a4: 'Disable or delete it on the API Keys page right away and create a new one. The old key stops working immediately.'
    },
    end: {
      eyebrow: 'Ready when you are',
      title: 'Ink is ready. Begin.',
      lead: 'An account takes a minute. The gateway handles the rest.',
      login: 'I have an account'
    }
  }
}

const zh: typeof en = {
  jackHome: {
    eyebrow: 'One key · Every model',
    titleLead: '一把密钥，',
    titleTail: '通达每一个模型。',
    lead: 'Claude、GPT、Gemini 收进同一个网关。换工具不必换密钥，换模型不必改代码——只要把 Base URL 指向这里。',
    ctaRegister: '免费创建账号',
    ctaLogin: '登录',
    ctaDashboard: '进入控制台',
    ctaGuide: '三分钟接入',
    protocolsLabel: '兼容',
    diagram: {
      label: '一把密钥经由网关路由到多个模型供应商',
      yourKey: 'YOUR KEY',
      gateway: '网关',
      more: '另有 {n} 个',
      models: '{n} 个模型',
      hint: '悬停供应商查看路由'
    },
    steps: {
      eyebrow: 'Three strokes',
      title: '三笔，就能落款。',
      lead: '不需要 SDK，不需要代理程序。你原本怎么调用模型，接下来就怎么调用。',
      s1: { title: '创建账号', desc: '注册后进入控制台，订阅、余额与用量一页看清。' },
      s2: { title: '生成密钥', desc: '在「API 密钥」页创建一把 sk- 密钥，选好分组即可使用。' },
      s3: { title: '指向这里', desc: '把工具的 Base URL 换成本站地址，其余设置原封不动。' }
    },
    code: {
      eyebrow: 'Drop-in',
      title1: '你熟悉的工具，',
      title2: '一行都不用学。',
      check1: 'Claude Code、Codex CLI 直接指向本站即可使用',
      check2: '任何 OpenAI / Anthropic 兼容 SDK 都能接入',
      check3: '流式输出、工具调用、长上下文照常工作',
      baseUrl: '你的 Base URL',
      copy: '复制',
      copied: '已复制',
      keyPlaceholder: 'sk-你的密钥',
      createKey: '创建密钥'
    },
    models: {
      eyebrow: 'Live catalogue',
      title: '此刻可用的模型。',
      lead: '数据直接来自模型广场，随站点配置实时更新。',
      count: '{models} 个模型 · {platforms} 个平台',
      all: '全部',
      more: '另有 {n} 个',
      viewAll: '查看完整价目'
    },
    features: {
      eyebrow: 'Why here',
      title: '安静、可靠，账目分明。',
      lead: '好的基础设施不抢戏。它只是一直在，而且每一笔都记得清清楚楚。',
      gateway: {
        eyebrow: 'Unified gateway',
        title: '一个端点，多种协议',
        desc: '同时兼容 Anthropic Messages、OpenAI Chat Completions 与 Responses。客户端说哪种语言，网关就回哪种语言。'
      },
      metered: { eyebrow: 'Metered', title: '实时计费', desc: '按实际 token 计价，每次请求即时入账。' },
      sticky: { eyebrow: 'Sticky session', title: '粘性会话', desc: '同一段对话固定走同一条上游，缓存命中更高，回复更一致。' },
      resilient: { eyebrow: 'Resilient', title: '账号池调度', desc: '多个上游账号自动排程，单点失效时无声切换，你的请求不必知道。' },
      transparent: { eyebrow: 'Transparent', title: '用量透明', desc: '每把密钥、每个模型、每一天的花费都摊在控制台，没有黑箱。', figure: '可追溯' }
    },
    faq: {
      eyebrow: 'FAQ',
      title: '常见问题',
      lead: '没找到答案？登录后查看文档，或直接联系管理员。',
      q1: '需要改代码吗？',
      a1: '不需要。只要把 Base URL 换成本站地址、API Key 换成你在控制台创建的 sk- 密钥，其余一律照旧。',
      q2: '支持哪些客户端？',
      a2: 'Claude Code、Codex CLI，以及任何支持 OpenAI 或 Anthropic 兼容端点的 SDK、编辑器插件与聊天前端。',
      q3: '费用怎么计算？',
      a3: '按实际 token 用量实时扣除，或使用订阅套餐的额度。每次请求的模型、用量与花费都能在「使用记录」中查到。',
      q4: '密钥不小心泄露了怎么办？',
      a4: '立刻到控制台的「API 密钥」页停用或删除该密钥，再创建一把新的即可，旧密钥会立即失效。'
    },
    end: {
      eyebrow: 'Ready when you are',
      title: '备好纸墨，就落笔吧。',
      lead: '创建账号只要一分钟。接下来的事，交给网关。',
      login: '已有账号，登录'
    }
  }
}

const zhHant: typeof en = {
  jackHome: {
    eyebrow: 'One key · Every model',
    titleLead: '一把金鑰，',
    titleTail: '通達每一個模型。',
    lead: 'Claude、GPT、Gemini 收進同一個閘道。換工具不必換金鑰，換模型不必改程式碼——只要把 Base URL 指向這裡。',
    ctaRegister: '免費建立帳號',
    ctaLogin: '登入',
    ctaDashboard: '進入控制台',
    ctaGuide: '三分鐘接入',
    protocolsLabel: '相容',
    diagram: {
      label: '一把金鑰經由閘道路由到多個模型供應商',
      yourKey: 'YOUR KEY',
      gateway: '閘道',
      more: '另有 {n} 個',
      models: '{n} 個模型',
      hint: '懸停供應商查看路由'
    },
    steps: {
      eyebrow: 'Three strokes',
      title: '三筆，就能落款。',
      lead: '不需要 SDK，不需要代理程式。你原本怎麼呼叫模型，接下來就怎麼呼叫。',
      s1: { title: '建立帳號', desc: '註冊後進入控制台，訂閱、餘額與用量一頁看清。' },
      s2: { title: '產生金鑰', desc: '在「API 金鑰」頁建立一把 sk- 金鑰，選好分組即可使用。' },
      s3: { title: '指向這裡', desc: '把工具的 Base URL 換成本站位址，其餘設定原封不動。' }
    },
    code: {
      eyebrow: 'Drop-in',
      title1: '你熟悉的工具，',
      title2: '一行都不用學。',
      check1: 'Claude Code、Codex CLI 直接指向本站即可使用',
      check2: '任何 OpenAI / Anthropic 相容 SDK 都能接入',
      check3: '串流、工具呼叫、長上下文照常運作',
      baseUrl: '你的 Base URL',
      copy: '複製',
      copied: '已複製',
      keyPlaceholder: 'sk-你的金鑰',
      createKey: '建立金鑰'
    },
    models: {
      eyebrow: 'Live catalogue',
      title: '此刻可用的模型。',
      lead: '資料直接來自模型廣場，隨站點設定即時更新。',
      count: '{models} 個模型 · {platforms} 個平台',
      all: '全部',
      more: '另有 {n} 個',
      viewAll: '查看完整價目'
    },
    features: {
      eyebrow: 'Why here',
      title: '安靜、可靠，帳目分明。',
      lead: '好的基礎設施不搶戲。它只是一直在，而且每一筆都記得清清楚楚。',
      gateway: {
        eyebrow: 'Unified gateway',
        title: '一個端點，多種協定',
        desc: '同時相容 Anthropic Messages、OpenAI Chat Completions 與 Responses。客戶端講哪種語言，閘道就回哪種語言。'
      },
      metered: { eyebrow: 'Metered', title: '即時計費', desc: '按實際 token 計價，每次請求即時入帳。' },
      sticky: { eyebrow: 'Sticky session', title: '黏性會話', desc: '同一段對話固定走同一條上游，快取命中更高，回應更一致。' },
      resilient: { eyebrow: 'Resilient', title: '帳號池調度', desc: '多個上游帳號自動排程，單點失效時無聲切換，你的請求不必知道。' },
      transparent: { eyebrow: 'Transparent', title: '用量透明', desc: '每把金鑰、每個模型、每一天的花費都攤在控制台，沒有黑箱。', figure: '可追溯' }
    },
    faq: {
      eyebrow: 'FAQ',
      title: '常見問題',
      lead: '沒找到答案？登入後查看文件，或直接聯絡管理員。',
      q1: '需要改程式碼嗎？',
      a1: '不需要。只要把 Base URL 換成本站位址、API Key 換成你在控制台建立的 sk- 金鑰，其餘一律照舊。',
      q2: '支援哪些客戶端？',
      a2: 'Claude Code、Codex CLI，以及任何支援 OpenAI 或 Anthropic 相容端點的 SDK、編輯器外掛與聊天前端。',
      q3: '費用怎麼計算？',
      a3: '依實際 token 用量即時扣除，或使用訂閱方案的額度。每次請求的模型、用量與花費都能在「使用紀錄」中查到。',
      q4: '金鑰不小心外洩了怎麼辦？',
      a4: '立刻到控制台的「API 金鑰」頁停用或刪除該金鑰，再建立一把新的即可，舊金鑰會立即失效。'
    },
    end: {
      eyebrow: 'Ready when you are',
      title: '備好紙墨，就落筆吧。',
      lead: '建立帳號只要一分鐘。接下來的事，交給閘道。',
      login: '已有帳號，登入'
    }
  }
}

export const jackHomeMessages = { en, zh, 'zh-Hant': zhHant }
