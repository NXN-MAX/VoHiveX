type LogRecord = Record<string, unknown>

const logFieldNames = new Set([
  'timestamp', 'time', 'datetime', 'created_at', 'createdAt', 'ts',
  'level', 'severity', 'log_level', 'logLevel',
  'device_id', 'deviceId', 'device', 'source', 'module', 'component',
  'message', 'msg', 'text', 'detail', 'event',
])

function firstText(row: LogRecord, keys: string[]) {
  for (const key of keys) {
    const value = row[key]
    if (value !== undefined && value !== null && typeof value !== 'object' && String(value).trim()) return String(value).trim()
  }
  return ''
}

function formatLogTime(value: string) {
  const number = Number(value)
  if (Number.isFinite(number)) {
    const date = new Date(number < 1e12 ? number * 1000 : number)
    if (!Number.isNaN(date.getTime())) return date.toISOString()
  }
  return value.replace(/\s+/, 'T')
}

export function normalizeLogEntry(value: unknown) {
  if (typeof value === 'string') return value
  if (value === null || value === undefined) return ''
  if (typeof value !== 'object' || Array.isArray(value)) return String(value)

  const row = value as LogRecord
  const time = formatLogTime(firstText(row, ['timestamp', 'time', 'datetime', 'created_at', 'createdAt', 'ts']))
  const level = firstText(row, ['level', 'severity', 'log_level', 'logLevel']).toUpperCase()
  const device = firstText(row, ['device_id', 'deviceId', 'device', 'source', 'module', 'component'])
  const message = firstText(row, ['message', 'msg', 'text', 'content', 'detail', 'event'])
  const fields = [time, level, device, message].filter(Boolean)
  if (fields.length) return fields.join(' ')

  try { return JSON.stringify(row) }
  catch { return String(value) }
}

function extractLogRows(value: unknown, depth = 0): unknown[] {
  if (depth > 6 || value === null || value === undefined) return []
  if (Array.isArray(value)) return value.flatMap((item) => extractLogRows(item, depth + 1))
  if (typeof value !== 'object') return String(value).split('\n')

  const record = value as LogRecord
  if (Object.entries(record).some(([key, field]) => logFieldNames.has(key) && field !== null && typeof field !== 'object')) return [record]

  const preferredKeys = ['lines', 'logs', 'items', 'entries', 'history', 'data', 'result', 'content']
  for (const key of preferredKeys) {
    if (!(key in record)) continue
    const rows = extractLogRows(record[key], depth + 1)
    if (rows.length) return rows
  }

  for (const nested of Object.values(record)) {
    if (Array.isArray(nested) || (nested && typeof nested === 'object')) {
      const rows = extractLogRows(nested, depth + 1)
      if (rows.length) return rows
    }
  }
  return [record]
}

export function normalizeLogResponse(data: unknown) {
  const rows = extractLogRows(data)
  return rows.map(normalizeLogEntry).filter((line) => line.length > 0)
}
