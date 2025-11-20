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
  fetchTableData()
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
      <section class="header-panel card">
        <div class="title-block">
          <p class="eyebrow">数据总览</p>
          <h2>{{ selectedTable || '请选择一个表' }}</h2>
          <p class="muted" v-if="selectedTable">
            {{ columnCount }} 列 · {{ pagination.total }} 行
          </p>
          <p class="muted" v-else>选择左侧任意表开始浏览</p>
          <div class="chip-row" v-if="selectedTable">
            <span class="chip">范围 {{ rangeLabel }}</span>
            <span class="chip">每页 {{ pagination.limit }}</span>
            <span class="chip">刷新于 {{ lastRefreshedText }}</span>
          </div>
        </div>
        <div class="toolbar-actions" v-if="selectedTable">
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
      </section>

      <section class="stats-grid" v-if="selectedTable">
        <article class="stat-card card">
          <span class="stat-label">总行数</span>
          <span class="stat-value">{{ pagination.total }}</span>
          <span class="stat-subtitle">范围 {{ rangeLabel }}</span>
        </article>
        <article class="stat-card card">
          <span class="stat-label">列数</span>
          <span class="stat-value">{{ columnCount }}</span>
          <span class="stat-subtitle">包含隐藏 `_rowid`</span>
        </article>
        <article class="stat-card card">
          <span class="stat-label">最近刷新</span>
          <span class="stat-value">{{ lastRefreshedText }}</span>
          <span class="stat-subtitle">
            <button
              class="link-button"
              @click="fetchTableData"
              :disabled="tableLoading"
            >
              重新拉取
            </button>
          </span>
        </article>
      </section>

      <div v-if="tableError" class="banner error">{{ tableError }}</div>

      <div v-if="tableLoading" class="loading-card card">加载数据中…</div>

      <div v-else-if="selectedTable && rows.length" class="table-card card">
        <div class="table-scroll">
          <table>
            <thead>
              <tr>
                <th v-for="col in columns" :key="col">{{ col }}</th>
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

      <div v-else-if="selectedTable" class="empty-state card">
        <h3>暂时没有数据</h3>
        <p>尝试新增一行或调整分页条件。</p>
        <button @click="openCreateModal">新增行</button>
      </div>

      <div v-else class="empty-state card">
        <h3>欢迎使用 sqliteviewer</h3>
        <p>从左侧选择一个表即可开始查看与编辑数据。</p>
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
