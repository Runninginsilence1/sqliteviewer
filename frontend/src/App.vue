<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'

const tables = ref([])
const tablesLoading = ref(false)
const selectedTable = ref('')
const tableError = ref('')
const tableLoading = ref(false)
const columns = ref([])
const rows = ref([])
const pagination = reactive({
  limit: 50,
  offset: 0,
  total: 0,
})

const editingRow = ref(null)
const savingEdit = ref(false)
const isCreating = ref(false)
const lastRefreshed = ref(null)

// Tab management
const activeTab = ref('data')
const tabs = ['data', 'schema', 'query']

// Search and sort
const searchQuery = ref('')
const sortColumn = ref('')
const sortDirection = ref('ASC')

// Schema view
const tableSchema = ref(null)
const schemaLoading = ref(false)

// SQL Query
const sqlQuery = ref('')
const queryResult = ref(null)
const queryLoading = ref(false)
const queryError = ref('')
const queryHistory = ref([])

const limitOptions = [25, 50, 100, 250]

const canPrev = computed(() => pagination.offset > 0)
const canNext = computed(
  () => pagination.offset + pagination.limit < pagination.total,
)

const rangeLabel = computed(() => {
  if (pagination.total === 0) return '0 / 0'
  const start = pagination.offset + 1
  const end = Math.min(pagination.offset + pagination.limit, pagination.total)
  return `${start}-${end} / ${pagination.total}`
})

const columnCount = computed(() =>
  columns.value.filter((col) => col !== '_rowid').length,
)
const lastRefreshedText = computed(() => {
  if (!lastRefreshed.value) return '尚未刷新'
  return lastRefreshed.value.toLocaleTimeString()
})

const fetchTables = async () => {
  tablesLoading.value = true
  tableError.value = ''
  try {
    const res = await fetch('/api/tables')
    if (!res.ok) throw new Error('无法获取数据表列表')
    const data = await res.json()
    tables.value = data.tables || []
    if (!selectedTable.value && tables.value.length) {
      selectedTable.value = tables.value[0]
    }
  } catch (err) {
    tableError.value = err.message || '加载数据表失败'
  } finally {
    tablesLoading.value = false
  }
}

const fetchTableData = async () => {
  if (!selectedTable.value) return
  tableLoading.value = true
  tableError.value = ''
  try {
    const params = new URLSearchParams({
      limit: String(pagination.limit),
      offset: String(pagination.offset),
    })
    if (searchQuery.value) {
      params.append('search', searchQuery.value)
    }
    if (sortColumn.value) {
      params.append('orderBy', sortColumn.value)
      params.append('orderDir', sortDirection.value)
    }
    const res = await fetch(`/api/tables/${selectedTable.value}?${params}`)
    if (!res.ok) throw new Error('无法加载表数据')
    const data = await res.json()
    columns.value = data.columns || []
    rows.value = data.rows || []
    pagination.total = data.total || 0
    lastRefreshed.value = new Date()
  } catch (err) {
    tableError.value = err.message || '加载表数据失败'
    columns.value = []
    rows.value = []
    pagination.total = 0
  } finally {
    tableLoading.value = false
  }
}

const fetchTableSchema = async () => {
  if (!selectedTable.value) return
  schemaLoading.value = true
  try {
    const res = await fetch(`/api/tables/${selectedTable.value}/schema`)
    if (!res.ok) throw new Error('无法加载表结构')
    tableSchema.value = await res.json()
  } catch (err) {
    tableError.value = err.message || '加载表结构失败'
  } finally {
    schemaLoading.value = false
  }
}

const executeQuery = async () => {
  if (!sqlQuery.value.trim()) return
  queryLoading.value = true
  queryError.value = ''
  queryResult.value = null
  try {
    const res = await fetch('/api/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: sqlQuery.value }),
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || '查询失败')
    }
    const data = await res.json()
    queryResult.value = data
    // Add to history
    if (!queryHistory.value.includes(sqlQuery.value)) {
      queryHistory.value.unshift(sqlQuery.value)
      if (queryHistory.value.length > 10) {
        queryHistory.value = queryHistory.value.slice(0, 10)
      }
    }
    // Refresh table data if it's a write operation
    if (data.type === 'write' && selectedTable.value) {
      await fetchTableData()
      await fetchTables()
    }
  } catch (err) {
    queryError.value = err.message || '执行查询失败'
  } finally {
    queryLoading.value = false
  }
}

const setSort = (col) => {
  if (sortColumn.value === col) {
    sortDirection.value = sortDirection.value === 'ASC' ? 'DESC' : 'ASC'
  } else {
    sortColumn.value = col
    sortDirection.value = 'ASC'
  }
  pagination.offset = 0
  fetchTableData()
}

const clearSearch = () => {
  searchQuery.value = ''
  pagination.offset = 0
  fetchTableData()
}

const selectTable = (table) => {
  if (table === selectedTable.value) return
  selectedTable.value = table
}

const changeLimit = (event) => {
  pagination.limit = Number(event.target.value)
  pagination.offset = 0
  fetchTableData()
}

const nextPage = () => {
  if (!canNext.value) return
  pagination.offset += pagination.limit
  fetchTableData()
}

const prevPage = () => {
  if (!canPrev.value) return
  pagination.offset = Math.max(0, pagination.offset - pagination.limit)
  fetchTableData()
}

const openEditor = (row) => {
  editingRow.value = JSON.parse(JSON.stringify(row))
  isCreating.value = false
}

const openCreateModal = () => {
  if (!columns.value.length) return
  const row = {}
  columns.value.forEach((col) => {
    if (col !== '_rowid') {
      row[col] = ''
    }
  })
  editingRow.value = row
  isCreating.value = true
}

const closeEditor = () => {
  editingRow.value = null
  isCreating.value = false
}

const saveRow = async () => {
  if (!editingRow.value || !selectedTable.value) return
  const payload = { ...editingRow.value }
  const rowid = payload._rowid
  delete payload._rowid
  savingEdit.value = true
  tableError.value = ''
  try {
    let res
    if (isCreating.value) {
      res = await fetch(`/api/tables/${selectedTable.value}/rows`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    } else {
      res = await fetch(
        `/api/tables/${selectedTable.value}/rows/${rowid}`,
        {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        },
      )
    }
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || '更新失败')
    }
    await fetchTableData()
    closeEditor()
  } catch (err) {
    tableError.value = err.message || '保存行失败'
  } finally {
    savingEdit.value = false
  }
}

const deleteRow = async (row) => {
  if (!selectedTable.value || !row?._rowid) return
  const ok = confirm(`确定删除行 #${row._rowid} 吗？`)
  if (!ok) return
  tableError.value = ''
  try {
    const res = await fetch(
      `/api/tables/${selectedTable.value}/rows/${row._rowid}`,
      {
        method: 'DELETE',
      },
    )
    if (!res.ok) {
      const err = await res.json().catch(() => ({}))
      throw new Error(err.error || '删除失败')
    }
    await fetchTableData()
  } catch (err) {
    tableError.value = err.message || '删除行失败'
  }
}

const downloadExport = (format) => {
  if (!selectedTable.value) return
  const url = `/api/tables/${selectedTable.value}/export?format=${format}`
  window.open(url, '_blank')
}

const formatCell = (value) => {
  if (value === null || value === undefined || value === '') return 'NULL'
  if (typeof value === 'object') {
    return JSON.stringify(value)
  }
  return value
}

watch(selectedTable, () => {
  pagination.offset = 0
  searchQuery.value = ''
  sortColumn.value = ''
  sortDirection.value = 'ASC'
  if (activeTab.value === 'data') {
    fetchTableData()
  } else if (activeTab.value === 'schema') {
    fetchTableSchema()
  }
})

watch(activeTab, (newTab) => {
  if (!selectedTable.value) return
  if (newTab === 'data') {
    fetchTableData()
  } else if (newTab === 'schema') {
    fetchTableSchema()
  }
})

onMounted(() => {
  fetchTables()
})
</script>

<template>
  <div class="app">
    <aside class="sidebar">
      <div class="sidebar-header">
  <div>
          <p class="eyebrow">SQLite Dashboard</p>
          <h1>sqliteviewer</h1>
        </div>
        <button class="ghost" @click="fetchTables" :disabled="tablesLoading">
          {{ tablesLoading ? '加载中…' : '刷新' }}
        </button>
      </div>
      <div class="table-list" v-if="tables.length">
        <button
          v-for="table in tables"
          :key="table"
          :class="['table-item', { active: table === selectedTable }]"
          @click="selectTable(table)"
        >
          {{ table }}
        </button>
      </div>
      <p v-else class="empty-tip">
        {{ tablesLoading ? '正在加载表...' : '未找到任何表' }}
      </p>
    </aside>

    <main class="content">
      <section class="header-panel card" v-if="selectedTable">
        <div class="title-block">
          <p class="eyebrow">数据总览</p>
          <h2>{{ selectedTable }}</h2>
          <p class="muted">
            {{ columnCount }} 列 · {{ pagination.total }} 行
          </p>
          <div class="chip-row">
            <span class="chip">范围 {{ rangeLabel }}</span>
            <span class="chip">每页 {{ pagination.limit }}</span>
            <span class="chip">刷新于 {{ lastRefreshedText }}</span>
          </div>
        </div>
      </section>

      <div v-if="!selectedTable" class="empty-state card">
        <h2>请选择一个表</h2>
        <p class="muted">从左侧列表中选择一个表开始浏览</p>
      </div>

      <div v-else class="tabs-container">
        <div class="tabs">
          <button
            v-for="tab in tabs"
            :key="tab"
            :class="['tab', { active: activeTab === tab }]"
            @click="activeTab = tab"
          >
            {{ tab === 'data' ? '数据' : tab === 'schema' ? '结构' : 'SQL 查询' }}
          </button>
        </div>

        <!-- Data Tab -->
        <div v-if="activeTab === 'data'" class="tab-content">
          <div class="toolbar card">
            <div class="search-box">
              <input
                v-model="searchQuery"
                @keyup.enter="fetchTableData"
                type="text"
                placeholder="搜索数据..."
                class="search-input"
              />
              <button v-if="searchQuery" @click="clearSearch" class="clear-btn">×</button>
            </div>
            <div class="toolbar-actions">
              <button @click="openCreateModal">新增行</button>
              <label class="select-wrap">
                每页
                <select :value="pagination.limit" @change="changeLimit">
                  <option v-for="limit in limitOptions" :key="limit" :value="limit">
                    {{ limit }}
                  </option>
                </select>
              </label>
              <div class="pagination-buttons">
                <button class="secondary" @click="prevPage" :disabled="!canPrev">
                  上一页
                </button>
                <button class="secondary" @click="nextPage" :disabled="!canNext">
                  下一页
                </button>
              </div>
              <div class="divider vertical" />
              <div class="export-buttons">
                <button class="secondary" @click="downloadExport('csv')">
              CSV
            </button>
            <button class="secondary" @click="downloadExport('json')">
              JSON
            </button>
            <button class="secondary" @click="downloadExport('sql')">
              SQL
            </button>
          </div>
        </div>
      </div>

          <div v-if="tableError" class="banner error">{{ tableError }}</div>

          <div v-if="tableLoading" class="loading-card card">加载数据中…</div>

          <div v-else-if="rows.length" class="table-card card">
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th
                      v-for="col in columns"
                      :key="col"
                      :class="{ sortable: col !== '_rowid' }"
                      @click="col !== '_rowid' && setSort(col)"
                    >
                      {{ col }}
                      <span
                        v-if="sortColumn === col"
                        class="sort-indicator"
                      >
                        {{ sortDirection === 'ASC' ? '↑' : '↓' }}
                      </span>
                    </th>
                    <th class="actions-head">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in rows" :key="row._rowid">
                    <td v-for="col in columns" :key="col">
                      <span class="cell-text">{{ formatCell(row[col]) }}</span>
                    </td>
                    <td class="actions-cell">
                      <button class="secondary" @click="openEditor(row)">
                        编辑
                      </button>
                      <button class="danger" @click="deleteRow(row)">删除</button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div v-else class="empty-state card">
            <h3>暂时没有数据</h3>
            <p>尝试新增一行或调整分页条件。</p>
            <button @click="openCreateModal">新增行</button>
          </div>
        </div>

        <!-- Schema Tab -->
        <div v-if="activeTab === 'schema'" class="tab-content">
          <div v-if="schemaLoading" class="loading-card card">加载结构中…</div>
          <div v-else-if="tableSchema" class="schema-card card">
            <h3>表结构</h3>
            <pre class="schema-sql">{{ tableSchema.schema }}</pre>

            <h3>列信息</h3>
            <table class="schema-table">
              <thead>
                <tr>
                  <th>列名</th>
                  <th>类型</th>
                  <th>非空</th>
                  <th>主键</th>
                  <th>默认值</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="col in tableSchema.columns" :key="col.cid">
                  <td><strong>{{ col.name }}</strong></td>
                  <td>{{ col.type }}</td>
                  <td>{{ col.notnull ? '是' : '否' }}</td>
                  <td>{{ col.primaryKey ? '是' : '否' }}</td>
                  <td>{{ col.default || 'NULL' }}</td>
                </tr>
              </tbody>
            </table>

            <h3 v-if="tableSchema.indexes?.length">索引</h3>
            <div v-if="tableSchema.indexes?.length" class="indexes-list">
              <div v-for="idx in tableSchema.indexes" :key="idx.name" class="index-item">
                <strong>{{ idx.name }}</strong>
                <pre class="index-sql">{{ idx.sql || '自动索引' }}</pre>
              </div>
            </div>
          </div>
        </div>

        <!-- Query Tab -->
        <div v-if="activeTab === 'query'" class="tab-content">
          <div class="query-editor card">
            <div class="query-toolbar">
              <button @click="executeQuery" :disabled="queryLoading || !sqlQuery.trim()">
                {{ queryLoading ? '执行中…' : '执行查询' }}
              </button>
              <button class="secondary" @click="sqlQuery = ''">清空</button>
              <div v-if="queryHistory.length" class="history-dropdown">
                <button class="secondary">历史查询 ▼</button>
                <div class="history-menu">
                  <div
                    v-for="(hist, idx) in queryHistory"
                    :key="idx"
                    class="history-item"
                    @click="sqlQuery = hist"
                  >
                    {{ hist.substring(0, 50) }}{{ hist.length > 50 ? '...' : '' }}
                  </div>
                </div>
              </div>
            </div>
            <textarea
              v-model="sqlQuery"
              class="sql-textarea"
              placeholder="输入 SQL 查询，例如：&#10;SELECT * FROM users WHERE age > 18;&#10;&#10;支持 SELECT、INSERT、UPDATE、DELETE 等操作"
              rows="10"
            ></textarea>
            <div v-if="queryError" class="banner error">{{ queryError }}</div>
            <div v-if="queryResult" class="query-result">
              <div v-if="queryResult.type === 'select'" class="result-table">
                <h4>查询结果 ({{ queryResult.rows?.length || 0 }} 行)</h4>
                <div class="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th v-for="col in queryResult.columns" :key="col">{{ col }}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="(row, idx) in queryResult.rows" :key="idx">
                        <td v-for="col in queryResult.columns" :key="col">
                          <span class="cell-text">{{ formatCell(row[col]) }}</span>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
              <div v-else class="result-info">
                <p>✓ 操作成功</p>
                <p v-if="queryResult.rowsAffected !== undefined">
                  影响行数: {{ queryResult.rowsAffected }}
                </p>
                <p v-if="queryResult.lastInsertId !== undefined && queryResult.lastInsertId > 0">
                  最后插入 ID: {{ queryResult.lastInsertId }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>

    <div v-if="editingRow" class="modal-backdrop">
      <div class="modal">
        <header>
          <h3>
            {{ isCreating ? '新增行' : `编辑行 #${editingRow._rowid}` }}
          </h3>
          <button class="ghost" @click="closeEditor">关闭</button>
        </header>
        <section class="modal-body">
          <div v-for="col in columns" :key="col" class="field">
            <label>{{ col }}</label>
            <textarea v-model="editingRow[col]" rows="2" />
          </div>
        </section>
        <footer>
          <button class="ghost" @click="closeEditor">取消</button>
          <button @click="saveRow" :disabled="savingEdit">
            {{ savingEdit ? '保存中…' : '保存' }}
          </button>
        </footer>
      </div>
    </div>
  </div>
</template>
