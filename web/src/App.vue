<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useStorage, useClipboard, useDebounceFn } from '@vueuse/core'

// 持久化存储与设置
const managementKey = useStorage('cpa_management_key', '')
const viewMode = useStorage('cpa_speedtest_view_mode', 'grid') // 'grid' | 'table'
const concurrency = useStorage('cpa_speedtest_concurrency', 20)
const activeProvider = useStorage('cpa_speedtest_provider_filter', 'all')
const activeStatus = useStorage('cpa_speedtest_status_filter', 'all')
const sortBy = useStorage('cpa_speedtest_sort_by', 'default')

// 状态
const loadingChannels = ref(false)
const testingAll = ref(false)
const searchQuery = ref('')
const channels = ref([])
const modelList = ref([]) // 结构: { key, channel_id, channel_name, provider, model, disabled }
const testResults = ref({}) // key -> { ttft_ms, total_ms, status, status_code, message, tested_at }
const testingKeys = ref(new Set()) // 正在测速的模型 key
const activeDetail = ref(null) // 详情弹窗
const showKeyConfig = ref(false)

const { copy, copied } = useClipboard()

// API 请求封装：优先调用 Management 路由，附带 Management Key
async function apiFetch(endpoint, options = {}) {
  const headers = { ...(options.headers || {}) }

  // 检查管理密钥：同源 localStorage 或者用户输入的 key
  let key = managementKey.value
  if (!key) {
    key = localStorage.getItem('management_key') || localStorage.getItem('key') || ''
    if (key) managementKey.value = key
  }
  if (key) {
    headers['Authorization'] = `Bearer ${key}`
    headers['X-Management-Key'] = key
  }

  // 尝试 Management API 路由与 Resource 路由后备
  const mgmtUrl = `/v0/management/plugins/cpa-models-speed-test${endpoint}`
  const resourceUrl = `/v0/resource/plugins/cpa-models-speed-test/api${endpoint}`

  try {
    const res = await fetch(mgmtUrl, { ...options, headers })
    if (res.status === 401 || res.status === 403) {
      // 鉴权失败，尝试无鉴权的资源路由或提示
      try {
        const res2 = await fetch(resourceUrl, { ...options, headers })
        if (res2.ok) return await res2.json()
      } catch (_) {}
      showKeyConfig.value = true
      throw new Error(`Management 鉴权失败 (${res.status})，请输入正确的管理密钥`)
    }
    if (!res.ok) {
      const errText = await res.text()
      throw new Error(`HTTP ${res.status}: ${errText || res.statusText}`)
    }
    return await res.json()
  } catch (err) {
    // 若 Management 路由 404，尝试 resourceUrl
    if (err.message && err.message.includes('404')) {
      const res = await fetch(resourceUrl, { ...options, headers })
      if (res.ok) return await res.json()
    }
    throw err
  }
}

// 获取配置的所有渠道和模型列表
async function loadChannelsAndModels() {
  loadingChannels.value = true
  try {
    const data = await apiFetch('/channels')
    channels.value = data.channels || []

    // 如果后端直接返回了展平的 models，直接使用；否则从 channels 聚合
    if (data.models && data.models.length > 0) {
      modelList.value = data.models.map(m => ({
        ...m,
        key: `${m.channel_id}::${m.model}`
      }))
    } else {
      const list = []
      for (const ch of channels.value) {
        const models = ch.models || []
        for (const m of models) {
          list.push({
            key: `${ch.id}::${m}`,
            channel_id: ch.id,
            channel_name: ch.name || ch.label || ch.id,
            provider: ch.provider || 'unknown',
            model: m,
            disabled: ch.disabled || false
          })
        }
      }
      modelList.value = list
    }
  } catch (err) {
    console.error('加载模型列表失败:', err)
  } finally {
    loadingChannels.value = false
  }
}

// 单独测速
async function testSingleModel(target) {
  if (testingKeys.value.has(target.key)) return
  testingKeys.value.add(target.key)

  try {
    const res = await apiFetch('/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        channel_id: target.channel_id,
        provider: target.provider,
        model: target.model
      })
    })

    if (res.ok && res.result) {
      testResults.value[target.key] = res.result
    } else {
      testResults.value[target.key] = {
        key: target.key,
        channel_id: target.channel_id,
        channel_name: target.channel_name,
        provider: target.provider,
        model: target.model,
        ttft_ms: -1,
        total_ms: 0,
        status: 'Error',
        status_code: 500,
        message: res.error?.message || '测速失败',
        tested_at: new Date().toLocaleTimeString()
      }
    }
  } catch (err) {
    testResults.value[target.key] = {
      key: target.key,
      channel_id: target.channel_id,
      channel_name: target.channel_name,
      provider: target.provider,
      model: target.model,
      ttft_ms: -1,
      total_ms: 0,
      status: 'Error',
      status_code: 0,
      message: err.message || '请求异常',
      tested_at: new Date().toLocaleTimeString()
    }
  } finally {
    testingKeys.value.delete(target.key)
  }
}

// 一键测速（后端代发 + Goroutine 全局高并发测试）
async function testAllModels() {
  if (testingAll.value || filteredModels.value.length === 0) return
  testingAll.value = true

  const targets = filteredModels.value
  targets.forEach(t => testingKeys.value.add(t.key))

  try {
    const res = await apiFetch('/batch-test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        concurrency: Number(concurrency.value) || 20,
        items: targets.map(t => ({
          channel_id: t.channel_id,
          provider: t.provider,
          model: t.model
        }))
      })
    })

    if (res.ok && Array.isArray(res.results)) {
      for (const item of res.results) {
        const key = item.key || `${item.channel_id}::${item.model}`
        testResults.value[key] = item
      }
    }
  } catch (err) {
    console.error('批量测速失败，转为并发回退单测:', err)
    // 若批量接口不可用，则前端并发调用单测接口
    const poolLimit = Number(concurrency.value) || 10
    const queue = [...targets]
    const workers = Array(poolLimit).fill(0).map(async () => {
      while (queue.length > 0) {
        const item = queue.shift()
        if (item) await testSingleModel(item)
      }
    })
    await Promise.all(workers)
  } finally {
    targets.forEach(t => testingKeys.value.delete(t.key))
    testingAll.value = false
  }
}

// 所有可用的 Provider 列表
const availableProviders = computed(() => {
  const set = new Set()
  for (const m of modelList.value) {
    if (m.provider) set.add(m.provider.toLowerCase())
  }
  return Array.from(set).sort()
})

// 过滤模型列表
const filteredModels = computed(() => {
  let list = modelList.value

  // Provider 过滤
  if (activeProvider.value !== 'all') {
    list = list.filter(m => (m.provider || '').toLowerCase() === activeProvider.value.toLowerCase())
  }

  // 状态过滤
  if (activeStatus.value !== 'all') {
    list = list.filter(m => {
      const res = testResults.value[m.key]
      if (activeStatus.value === 'untested') return !res
      if (activeStatus.value === 'tested') return !!res
      if (activeStatus.value === 'success') return res && res.status === 'Success'
      if (activeStatus.value === 'quota') return res && res.status === 'Quota Limit'
      if (activeStatus.value === 'error') return res && res.status === 'Error'
      return true
    })
  }

  // 搜索关键词
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter(m =>
      m.model.toLowerCase().includes(q) ||
      (m.channel_name && m.channel_name.toLowerCase().includes(q)) ||
      (m.provider && m.provider.toLowerCase().includes(q))
    )
  }

  // 排序
  const sorted = [...list]
  if (sortBy.value === 'delay_asc') {
    sorted.sort((a, b) => {
      const ra = testResults.value[a.key]
      const rb = testResults.value[b.key]
      const da = ra && ra.status === 'Success' ? ra.ttft_ms : 999999
      const db = rb && rb.status === 'Success' ? rb.ttft_ms : 999999
      return da - db
    })
  } else if (sortBy.value === 'delay_desc') {
    sorted.sort((a, b) => {
      const ra = testResults.value[a.key]
      const rb = testResults.value[b.key]
      const da = ra && ra.status === 'Success' ? ra.ttft_ms : -1
      const db = rb && rb.status === 'Success' ? rb.ttft_ms : -1
      return db - da
    })
  } else if (sortBy.value === 'model_asc') {
    sorted.sort((a, b) => a.model.localeCompare(b.model))
  }

  return sorted
})

// 统计看板数据
const stats = computed(() => {
  let total = modelList.value.length
  let tested = 0
  let success = 0
  let quota = 0
  let error = 0
  let sumTTFT = 0
  let fastest = null

  for (const m of modelList.value) {
    const res = testResults.value[m.key]
    if (!res) continue
    tested++
    if (res.status === 'Success') {
      success++
      sumTTFT += res.ttft_ms
      if (!fastest || res.ttft_ms < fastest.ttft_ms) {
        fastest = { ...m, ...res }
      }
    } else if (res.status === 'Quota Limit') {
      quota++
    } else {
      error++
    }
  }

  return {
    total,
    tested,
    success,
    quota,
    error,
    avgTTFT: success > 0 ? Math.round(sumTTFT / success) : 0,
    fastest
  }
})

// 延迟等级与颜色高亮计算
function getLatencyClass(result) {
  if (!result) return 'status-untested'
  if (result.status === 'Quota Limit') return 'status-quota'
  if (result.status !== 'Success') return 'status-error'

  const ms = result.ttft_ms
  if (ms < 800) return 'status-excellent' // 优: 绿色
  if (ms <= 1500) return 'status-good'     // 良: 黄色
  return 'status-poor'                    // 差: 橙色
}

function getProviderColor(provider = '') {
  const p = provider.toLowerCase()
  if (p.includes('openai') || p.includes('codex') || p.includes('chatgpt')) return '#10b981'
  if (p.includes('claude') || p.includes('anthropic')) return '#d97706'
  if (p.includes('gemini') || p.includes('google')) return '#0ea5e9'
  if (p.includes('deepseek')) return '#3b82f6'
  if (p.includes('groq')) return '#f43f5e'
  if (p.includes('qwen') || p.includes('ali')) return '#8b5cf6'
  return '#64748b'
}

// 导出测试报告 (Markdown 格式)
function exportMarkdownReport() {
  const rows = [
    '| 渠道账号 | 类型 | 模型名称 | 首字延迟 (TTFT) | 状态 | 详情 |',
    '| :--- | :--- | :--- | :--- | :--- | :--- |'
  ]
  for (const m of modelList.value) {
    const res = testResults.value[m.key]
    const ttft = res && res.status === 'Success' ? `${res.ttft_ms} ms` : '-'
    const status = res ? res.status : '未测试'
    const msg = res ? (res.message || 'OK') : '-'
    rows.push(`| ${m.channel_name} | ${m.provider} | \`${m.model}\` | **${ttft}** | ${status} | ${msg} |`)
  }
  const md = `# CPA 模型延迟测速报告\n- 生成时间: ${new Date().toLocaleString()}\n- 总模型数: ${stats.value.total} | 成功: ${stats.value.success} | 限流: ${stats.value.quota} | 失败: ${stats.value.error} | 平均 TTFT: ${stats.value.avgTTFT} ms\n\n` + rows.join('\n')
  copy(md)
}

// 结合 WebMCP: 将核心状态与方法标准化暴露给浏览器端 AI Agent 及控制台调试
function setupWebMCP() {
  const webMCP = {
    version: '1.0.0',
    description: 'CPA Models Speed Test WebMCP Interface',
    tools: [
      {
        name: 'speedtest.run_all',
        description: '触发一键批量并发测速',
        execute: async () => {
          await testAllModels()
          return stats.value
        }
      },
      {
        name: 'speedtest.run_single',
        description: '对指定渠道和模型进行单独测速',
        parameters: { channel_id: 'string', model: 'string' },
        execute: async (args) => {
          const target = modelList.value.find(m =>
            (m.channel_id === args.channel_id || m.channel_name === args.channel_id) &&
            m.model === args.model
          )
          if (!target) throw new Error(`未找到匹配的模型: ${JSON.stringify(args)}`)
          await testSingleModel(target)
          return testResults.value[target.key]
        }
      },
      {
        name: 'speedtest.get_models',
        description: '获取当前配置的所有渠道和模型列表',
        execute: () => modelList.value
      },
      {
        name: 'speedtest.get_results',
        description: '获取当前所有模型的测速结果',
        execute: () => testResults.value
      },
      {
        name: 'speedtest.get_stats',
        description: '获取当前测速统计看板指标',
        execute: () => stats.value
      }
    ],
    // 便捷直接方法
    runAll: testAllModels,
    runSingle: (channelId, model) => {
      const target = modelList.value.find(m => (m.channel_id === channelId || m.channel_name === channelId) && m.model === model)
      if (target) return testSingleModel(target)
      console.warn('未找到指定模型:', channelId, model)
    },
    getStats: () => stats.value,
    getResults: () => testResults.value
  }

  // 挂载至全局 window
  window.__web_mcp__ = webMCP
  console.log('%c[WebMCP] CPA 模型测速调试接口已就绪. 输入 window.__web_mcp__ 查看能力', 'color: #10b981; font-weight: bold;')
}

onMounted(() => {
  loadChannelsAndModels()
  setupWebMCP()
})
</script>

<template>
  <div class="speedtest-app">
    <!-- 顶部导航栏 -->
    <header class="top-nav">
      <div class="brand">
        <div class="logo-badge">⚡</div>
        <div>
          <h1 class="brand-title">CPA 模型延迟测速</h1>
          <p class="brand-subtitle">轻量流式首字延迟探测 (TTFT) • 高并发渠道基准测试</p>
        </div>
      </div>

      <div class="nav-actions">
        <!-- 一键测速主按钮 -->
        <button
          class="btn btn-primary"
          :disabled="testingAll || loadingChannels || filteredModels.length === 0"
          @click="testAllModels"
        >
          <span v-if="testingAll" class="spinner"></span>
          <span v-else class="icon">⚡</span>
          <span>{{ testingAll ? '正在并发测速...' : '一键测速' }}</span>
        </button>

        <!-- 刷新列表 -->
        <button class="btn btn-secondary" :disabled="loadingChannels" @click="loadChannelsAndModels">
          <span class="icon">🔄</span>
          <span>刷新</span>
        </button>

        <!-- 导出报告 -->
        <button class="btn btn-secondary" :disabled="stats.tested === 0" @click="exportMarkdownReport">
          <span class="icon">📋</span>
          <span>{{ copied ? '已复制报告!' : '导出报告' }}</span>
        </button>

        <!-- 设置鉴权 -->
        <button class="btn btn-icon" :class="{ 'has-key': !!managementKey }" title="管理密钥设置" @click="showKeyConfig = !showKeyConfig">
          🔑
        </button>
      </div>
    </header>

    <!-- 管理密钥弹层 / 面板 -->
    <div v-if="showKeyConfig" class="key-drawer">
      <div class="key-drawer-inner">
        <div class="key-drawer-header">
          <span>⚙️ 管理接口鉴权 (Management Key)</span>
          <button class="btn-close" @click="showKeyConfig = false">✕</button>
        </div>
        <p class="key-drawer-desc">
          同源模式下会自动复用 CPA 管理面板登录密钥。如需手动指定密钥或跨源调试，请在此处填写：
        </p>
        <div class="key-input-row">
          <input
            v-model="managementKey"
            type="password"
            placeholder="输入 CPA 管理密钥 (Authorization Bearer / X-Management-Key)"
            class="input-field"
          />
          <button class="btn btn-sm btn-primary" @click="loadChannelsAndModels(); showKeyConfig = false">
            保存并刷新
          </button>
        </div>
      </div>
    </div>

    <!-- 概览看板数据条 (类似 Clash 仪表盘) -->
    <section class="stats-bar">
      <div class="stat-card">
        <span class="stat-label">总模型数</span>
        <span class="stat-value">{{ stats.total }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">已完成测速</span>
        <span class="stat-value text-cyan">{{ stats.tested }} / {{ stats.total }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">极速 / 良好</span>
        <span class="stat-value text-green">{{ stats.success }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">触发限流 (429)</span>
        <span class="stat-value text-purple">{{ stats.quota }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">错误 / 超时</span>
        <span class="stat-value text-red">{{ stats.error }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">平均 TTFT</span>
        <span class="stat-value text-yellow">
          {{ stats.avgTTFT > 0 ? stats.avgTTFT + ' ms' : '-' }}
        </span>
      </div>
      <div class="stat-card stat-fastest" v-if="stats.fastest">
        <span class="stat-label">👑 最优节点</span>
        <span class="stat-value text-green">
          {{ stats.fastest.ttft_ms }} ms
          <small class="fastest-model-name">{{ stats.fastest.model }}</small>
        </span>
      </div>
    </section>

    <!-- 控制与筛选工具栏 -->
    <section class="toolbar">
      <!-- 搜索框 -->
      <div class="search-box">
        <span class="search-icon">🔍</span>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索模型名称、渠道账号、提供方类型..."
          class="search-input"
        />
        <button v-if="searchQuery" class="clear-search" @click="searchQuery = ''">✕</button>
      </div>

      <!-- Provider 渠道类型筛选 Chips -->
      <div class="filter-chips">
        <button
          class="chip"
          :class="{ active: activeProvider === 'all' }"
          @click="activeProvider = 'all'"
        >
          全部渠道 ({{ modelList.length }})
        </button>
        <button
          v-for="p in availableProviders"
          :key="p"
          class="chip"
          :class="{ active: activeProvider === p }"
          @click="activeProvider = p"
        >
          <span class="provider-dot" :style="{ backgroundColor: getProviderColor(p) }"></span>
          {{ p.toUpperCase() }}
        </button>
      </div>

      <!-- 状态筛选 Chips -->
      <div class="filter-chips">
        <button class="chip" :class="{ active: activeStatus === 'all' }" @click="activeStatus = 'all'">全部状态</button>
        <button class="chip" :class="{ active: activeStatus === 'success' }" @click="activeStatus = 'success'">正常 ({{ stats.success }})</button>
        <button class="chip" :class="{ active: activeStatus === 'quota' }" @click="activeStatus = 'quota'">限流 ({{ stats.quota }})</button>
        <button class="chip" :class="{ active: activeStatus === 'error' }" @click="activeStatus = 'error'">失败 ({{ stats.error }})</button>
        <button class="chip" :class="{ active: activeStatus === 'untested' }" @click="activeStatus = 'untested'">未测 ({{ stats.total - stats.tested }})</button>
      </div>

      <!-- 排序与视图切换 -->
      <div class="view-and-sort">
        <select v-model="sortBy" class="sort-select">
          <option value="default">默认渠道排序</option>
          <option value="delay_asc">延迟从低到高 ⚡</option>
          <option value="delay_desc">延迟从高到低</option>
          <option value="model_asc">模型名称 A-Z</option>
        </select>

        <div class="view-toggle">
          <button
            class="toggle-btn"
            :class="{ active: viewMode === 'grid' }"
            title="卡片网格视图"
            @click="viewMode = 'grid'"
          >
            ⊞
          </button>
          <button
            class="toggle-btn"
            :class="{ active: viewMode === 'table' }"
            title="紧凑列表视图"
            @click="viewMode = 'table'"
          >
            ☰
          </button>
        </div>
      </div>
    </section>

    <!-- 加载中提示 -->
    <div v-if="loadingChannels" class="state-container">
      <div class="spinner-large"></div>
      <p>正在读取后端配置的所有上游渠道与模型...</p>
    </div>

    <!-- 空列表提示 -->
    <div v-else-if="filteredModels.length === 0" class="state-container">
      <span class="empty-icon">📂</span>
      <p>未找到符合条件的模型</p>
      <small v-if="modelList.length === 0">当前后端暂未配置任何渠道或凭证，请先在 CPA 中添加 Provider 凭证</small>
    </div>

    <!-- 卡片网格视图 (类似 Clash/v2rayN 节点卡片) -->
    <main v-else-if="viewMode === 'grid'" class="model-grid">
      <div
        v-for="item in filteredModels"
        :key="item.key"
        class="model-card"
        :class="[getLatencyClass(testResults[item.key]), { testing: testingKeys.has(item.key) }]"
      >
        <!-- 卡片顶部：渠道与提供商标识 -->
        <div class="card-header">
          <div class="provider-tag" :style="{ backgroundColor: getProviderColor(item.provider) + '22', color: getProviderColor(item.provider), borderColor: getProviderColor(item.provider) + '55' }">
            <span class="provider-dot" :style="{ backgroundColor: getProviderColor(item.provider) }"></span>
            {{ item.provider.toUpperCase() }}
          </div>
          <span class="channel-name" :title="item.channel_name">{{ item.channel_name }}</span>
        </div>

        <!-- 卡片中部：模型名称与复制 -->
        <div class="card-body">
          <div class="model-title-wrap">
            <h3 class="model-title" :title="item.model">{{ item.model }}</h3>
            <button class="copy-btn" title="复制模型名" @click="copy(item.model)">⧉</button>
          </div>

          <!-- 核心指标呈现：首字延迟 (TTFT) 与状态高亮 -->
          <div class="latency-display">
            <!-- 测速中动画 -->
            <div v-if="testingKeys.has(item.key)" class="latency-badge testing">
              <span class="pulse-dot"></span>
              <span>探测首字...</span>
            </div>

            <!-- 已测出结果 -->
            <div
              v-else-if="testResults[item.key]"
              class="latency-badge"
              :class="getLatencyClass(testResults[item.key])"
              @click="activeDetail = testResults[item.key]"
            >
              <template v-if="testResults[item.key].status === 'Success'">
                <span class="ttft-num">{{ testResults[item.key].ttft_ms }}</span>
                <span class="ttft-unit">ms</span>
              </template>
              <template v-else-if="testResults[item.key].status === 'Quota Limit'">
                <span>限流 (429)</span>
              </template>
              <template v-else>
                <span>失败 (Error)</span>
              </template>
            </div>

            <!-- 未测速 -->
            <div v-else class="latency-badge untested">
              <span>--- ms</span>
            </div>
          </div>
        </div>

        <!-- 卡片底部：操作与状态详情 -->
        <div class="card-footer">
          <span class="status-msg" :title="testResults[item.key]?.message || ''">
            <template v-if="testResults[item.key]">
              {{ testResults[item.key].status === 'Success' ? '✓ 连接正常' : (testResults[item.key].message || testResults[item.key].status) }}
            </template>
            <template v-else>
              未测试
            </template>
          </span>

          <!-- 单独测速按钮 -->
          <button
            class="btn-test-single"
            :disabled="testingKeys.has(item.key) || testingAll"
            title="单独测速此模型"
            @click="testSingleModel(item)"
          >
            <span v-if="testingKeys.has(item.key)" class="spinner-sm"></span>
            <span v-else>测速</span>
          </button>
        </div>
      </div>
    </main>

    <!-- 紧凑表格视图 -->
    <main v-else class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>渠道账号</th>
            <th>类型</th>
            <th>模型名称</th>
            <th>首字延迟 (TTFT)</th>
            <th>状态</th>
            <th>最近测速</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in filteredModels"
            :key="item.key"
            :class="getLatencyClass(testResults[item.key])"
          >
            <td class="cell-channel">{{ item.channel_name }}</td>
            <td>
              <span class="provider-pill" :style="{ color: getProviderColor(item.provider), borderColor: getProviderColor(item.provider) + '44' }">
                {{ item.provider }}
              </span>
            </td>
            <td class="cell-model">
              <code>{{ item.model }}</code>
            </td>
            <td>
              <span v-if="testingKeys.has(item.key)" class="text-cyan">探测中...</span>
              <span v-else-if="testResults[item.key]?.status === 'Success'" class="ttft-badge-table" :class="getLatencyClass(testResults[item.key])">
                ⚡ {{ testResults[item.key].ttft_ms }} ms
              </span>
              <span v-else-if="testResults[item.key]?.status === 'Quota Limit'" class="text-purple">
                ⏳ 限流 429
              </span>
              <span v-else-if="testResults[item.key]?.status === 'Error'" class="text-red">
                ✖ 失败
              </span>
              <span v-else class="text-muted">---</span>
            </td>
            <td>
              <span class="status-cell" :class="getLatencyClass(testResults[item.key])">
                {{ testResults[item.key]?.status || '未测试' }}
              </span>
            </td>
            <td class="text-muted text-sm">
              {{ testResults[item.key]?.tested_at || '-' }}
            </td>
            <td>
              <button
                class="btn-table-test"
                :disabled="testingKeys.has(item.key) || testingAll"
                @click="testSingleModel(item)"
              >
                {{ testingKeys.has(item.key) ? '测速中' : '测速' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </main>

    <!-- 结果详情模态框 -->
    <div v-if="activeDetail" class="modal-backdrop" @click="activeDetail = null">
      <div class="modal-dialog" @click.stop>
        <div class="modal-header">
          <h3>测速详情: {{ activeDetail.model }}</h3>
          <button class="btn-close" @click="activeDetail = null">✕</button>
        </div>
        <div class="modal-body">
          <div class="detail-row">
            <span class="detail-label">渠道账号:</span>
            <span class="detail-val">{{ activeDetail.channel_name }} ({{ activeDetail.channel_id }})</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">提供方类型:</span>
            <span class="detail-val">{{ activeDetail.provider }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">探测首字延迟 (TTFT):</span>
            <span class="detail-val" :class="getLatencyClass(activeDetail)">
              {{ activeDetail.ttft_ms >= 0 ? activeDetail.ttft_ms + ' ms' : '无数据' }}
            </span>
          </div>
          <div class="detail-row">
            <span class="detail-label">总响应耗时:</span>
            <span class="detail-val">{{ activeDetail.total_ms }} ms</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">状态判定:</span>
            <span class="detail-val" :class="getLatencyClass(activeDetail)">{{ activeDetail.status }} (HTTP {{ activeDetail.status_code || '-' }})</span>
          </div>
          <div v-if="activeDetail.message" class="detail-row">
            <span class="detail-label">响应/错误信息:</span>
            <pre class="error-pre">{{ activeDetail.message }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.speedtest-app {
  max-width: 1440px;
  margin: 0 auto;
  padding: 1.5rem;
  font-family: inherit;
}

/* 顶部导航 */
.top-nav {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
  padding-bottom: 1.25rem;
  border-bottom: 1px solid #1e293b;
  flex-wrap: wrap;
  gap: 1rem;
}

.brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.logo-badge {
  font-size: 1.5rem;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #3b82f6 0%, #10b981 100%);
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.25);
}

.brand-title {
  font-size: 1.35rem;
  font-weight: 700;
  color: #f8fafc;
  letter-spacing: -0.025em;
}

.brand-subtitle {
  font-size: 0.8rem;
  color: #94a3b8;
  margin-top: 0.15rem;
}

.nav-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

/* 按钮样式 */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.55rem 1rem;
  font-size: 0.875rem;
  font-weight: 600;
  border-radius: 8px;
  cursor: pointer;
  border: 1px solid transparent;
  transition: all 0.15s ease-in-out;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  color: #ffffff;
  box-shadow: 0 4px 14px rgba(16, 185, 129, 0.35);
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 6px 18px rgba(16, 185, 129, 0.45);
}

.btn-secondary {
  background: #1e293b;
  color: #cbd5e1;
  border-color: #334155;
}

.btn-secondary:hover:not(:disabled) {
  background: #334155;
  color: #f8fafc;
}

.btn-icon {
  background: #1e293b;
  border: 1px solid #334155;
  width: 38px;
  height: 38px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.btn-icon.has-key {
  border-color: #10b981;
}

/* 密钥设置抽屉 */
.key-drawer {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 12px;
  padding: 1rem 1.25rem;
  margin-bottom: 1.5rem;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
}

.key-drawer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
  margin-bottom: 0.5rem;
}

.key-drawer-desc {
  font-size: 0.8rem;
  color: #94a3b8;
  margin-bottom: 0.75rem;
}

.key-input-row {
  display: flex;
  gap: 0.75rem;
}

.input-field {
  flex: 1;
  background: #0f172a;
  border: 1px solid #334155;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  color: #f8fafc;
  font-size: 0.85rem;
}

.input-field:focus {
  outline: none;
  border-color: #3b82f6;
}

/* 看板指标条 */
.stats-bar {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 0.85rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 10px;
  padding: 0.85rem 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.stat-label {
  font-size: 0.75rem;
  color: #94a3b8;
  font-weight: 500;
}

.stat-value {
  font-size: 1.25rem;
  font-weight: 700;
  color: #f8fafc;
}

.stat-fastest {
  grid-column: span 2;
}

.fastest-model-name {
  display: block;
  font-size: 0.7rem;
  font-weight: normal;
  color: #94a3b8;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.text-cyan { color: #06b6d4; }
.text-green { color: #10b981; }
.text-yellow { color: #f59e0b; }
.text-purple { color: #c084fc; }
.text-red { color: #ef4444; }

/* 工具与筛选栏 */
.toolbar {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
  margin-bottom: 1.5rem;
  background: #131d31;
  border: 1px solid #1e293b;
  padding: 1rem;
  border-radius: 12px;
}

.search-box {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 0.85rem;
  font-size: 0.9rem;
  color: #64748b;
}

.search-input {
  width: 100%;
  padding: 0.6rem 2.2rem 0.6rem 2.4rem;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 8px;
  color: #f8fafc;
  font-size: 0.875rem;
}

.search-input:focus {
  outline: none;
  border-color: #3b82f6;
}

.clear-search {
  position: absolute;
  right: 0.75rem;
  background: none;
  border: none;
  color: #94a3b8;
  cursor: pointer;
}

.filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 0.75rem;
  font-size: 0.775rem;
  font-weight: 500;
  border-radius: 20px;
  background: #1e293b;
  color: #94a3b8;
  border: 1px solid #334155;
  cursor: pointer;
  transition: all 0.15s ease;
}

.chip:hover {
  color: #f8fafc;
  background: #273549;
}

.chip.active {
  background: #2563eb;
  color: #ffffff;
  border-color: #3b82f6;
}

.provider-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
}

.view-and-sort {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  margin-top: 0.25rem;
}

.sort-select {
  background: #0f172a;
  border: 1px solid #334155;
  color: #cbd5e1;
  padding: 0.45rem 0.75rem;
  border-radius: 6px;
  font-size: 0.8rem;
  cursor: pointer;
}

.view-toggle {
  display: flex;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 6px;
  overflow: hidden;
}

.toggle-btn {
  background: none;
  border: none;
  color: #64748b;
  padding: 0.35rem 0.65rem;
  font-size: 0.95rem;
  cursor: pointer;
}

.toggle-btn.active {
  background: #1e293b;
  color: #38bdf8;
}

/* 状态展示与空状态 */
.state-container {
  padding: 4rem 1rem;
  text-align: center;
  color: #94a3b8;
}

.empty-icon {
  font-size: 2.5rem;
  display: block;
  margin-bottom: 0.5rem;
}

/* 卡片网格布局 (类似 Clash / v2rayN 节点卡片) */
.model-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(290px, 1fr));
  gap: 1rem;
}

.model-card {
  background: #182234;
  border: 1px solid #28354b;
  border-radius: 12px;
  padding: 1.1rem;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 0.9rem;
  transition: all 0.2s ease-in-out;
  position: relative;
  overflow: hidden;
}

.model-card:hover {
  transform: translateY(-2px);
  border-color: #3b82f6;
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.35);
}

.model-card.status-excellent {
  border-left: 4px solid #10b981;
}

.model-card.status-good {
  border-left: 4px solid #f59e0b;
}

.model-card.status-poor {
  border-left: 4px solid #f97316;
}

.model-card.status-quota {
  border-left: 4px solid #c084fc;
}

.model-card.status-error {
  border-left: 4px solid #ef4444;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
}

.provider-tag {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 700;
  border: 1px solid;
}

.channel-name {
  font-size: 0.75rem;
  color: #94a3b8;
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}

.model-title-wrap {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.model-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: #f1f5f9;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copy-btn {
  background: none;
  border: none;
  color: #64748b;
  cursor: pointer;
  font-size: 0.85rem;
}

.copy-btn:hover {
  color: #38bdf8;
}

/* 核心延迟 Badge 高亮呈现 */
.latency-display {
  display: flex;
  align-items: center;
}

.latency-badge {
  display: inline-flex;
  align-items: baseline;
  gap: 0.25rem;
  padding: 0.4rem 0.85rem;
  border-radius: 8px;
  font-weight: 700;
  font-size: 1.15rem;
  cursor: pointer;
  border: 1px solid transparent;
}

.latency-badge.status-excellent {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
  border-color: rgba(16, 185, 129, 0.4);
  box-shadow: 0 0 12px rgba(16, 185, 129, 0.2);
}

.latency-badge.status-good {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.4);
}

.latency-badge.status-poor {
  background: rgba(249, 115, 22, 0.15);
  color: #f97316;
  border-color: rgba(249, 115, 22, 0.4);
}

.latency-badge.status-quota {
  background: rgba(192, 132, 252, 0.15);
  color: #c084fc;
  border-color: rgba(192, 132, 252, 0.4);
  font-size: 0.85rem;
}

.latency-badge.status-error {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
  border-color: rgba(239, 68, 68, 0.4);
  font-size: 0.85rem;
}

.latency-badge.untested {
  background: #1e293b;
  color: #64748b;
  font-size: 0.95rem;
}

.latency-badge.testing {
  background: rgba(6, 182, 212, 0.15);
  color: #06b6d4;
  border-color: rgba(6, 182, 212, 0.4);
  font-size: 0.85rem;
}

.ttft-num {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.ttft-unit {
  font-size: 0.75rem;
  font-weight: normal;
  color: inherit;
  opacity: 0.8;
}

.pulse-dot {
  width: 8px;
  height: 8px;
  background-color: #06b6d4;
  border-radius: 50%;
  animation: pulse 1s infinite alternate;
}

@keyframes pulse {
  0% { transform: scale(0.8); opacity: 0.6; }
  100% { transform: scale(1.3); opacity: 1; }
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 0.65rem;
  border-top: 1px solid #1e293b;
  gap: 0.5rem;
}

.status-msg {
  font-size: 0.75rem;
  color: #64748b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 170px;
}

.btn-test-single {
  background: #233047;
  color: #38bdf8;
  border: 1px solid #334155;
  padding: 0.35rem 0.75rem;
  border-radius: 6px;
  font-size: 0.775rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-test-single:hover:not(:disabled) {
  background: #0284c7;
  color: #ffffff;
  border-color: #0284c7;
}

/* 列表表格视图 */
.table-container {
  background: #182234;
  border: 1px solid #28354b;
  border-radius: 12px;
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
  text-align: left;
}

.data-table th {
  background: #131d31;
  color: #94a3b8;
  padding: 0.75rem 1rem;
  font-weight: 600;
  border-bottom: 1px solid #28354b;
}

.data-table td {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid #1f2c41;
  color: #cbd5e1;
}

.data-table tr:hover {
  background: #1d2b42;
}

.provider-pill {
  border: 1px solid;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
}

.ttft-badge-table {
  font-weight: 700;
  font-family: ui-monospace, monospace;
}

.ttft-badge-table.status-excellent { color: #10b981; }
.ttft-badge-table.status-good { color: #f59e0b; }
.ttft-badge-table.status-poor { color: #f97316; }

.btn-table-test {
  background: #233047;
  color: #38bdf8;
  border: 1px solid #334155;
  padding: 0.25rem 0.6rem;
  border-radius: 4px;
  font-size: 0.75rem;
  cursor: pointer;
}

.btn-table-test:hover:not(:disabled) {
  background: #0284c7;
  color: #fff;
}

/* 模态框 */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  padding: 1rem;
}

.modal-dialog {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 12px;
  width: 100%;
  max-width: 560px;
  padding: 1.5rem;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.25rem;
}

.btn-close {
  background: none;
  border: none;
  color: #94a3b8;
  font-size: 1.2rem;
  cursor: pointer;
}

.detail-row {
  margin-bottom: 0.75rem;
  font-size: 0.85rem;
}

.detail-label {
  color: #94a3b8;
  margin-right: 0.5rem;
}

.detail-val {
  color: #f1f5f9;
  font-weight: 600;
}

.error-pre {
  margin-top: 0.5rem;
  background: #0f172a;
  padding: 0.75rem;
  border-radius: 6px;
  color: #ef4444;
  font-size: 0.75rem;
  max-height: 180px;
  overflow: auto;
  white-space: pre-wrap;
}

/* 旋转加载器 */
.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #ffffff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  display: inline-block;
}

.spinner-sm {
  width: 12px;
  height: 12px;
  border: 2px solid rgba(56, 189, 248, 0.3);
  border-top-color: #38bdf8;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  display: inline-block;
}

.spinner-large {
  width: 36px;
  height: 36px;
  border: 3px solid rgba(56, 189, 248, 0.2);
  border-top-color: #38bdf8;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin: 0 auto 1rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
