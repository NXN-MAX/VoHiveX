type LogRecord = Record<string, unknown>

function firstText(row: LogRecord, keys: string[]) {
  for (const key of keys) {
    const value = row[key]
    if (value !== undefined && value !== null && String(value).trim()) return String(value).trim()
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

export function normalizeLogResponse(data: unknown) {
  let rows: unknown[]
  if (Array.isArray(data)) rows = data
  else if (data && typeof data === 'object') {
    const record = data as LogRecord
    const nested = ['lines', 'logs', 'items', 'entries'].map((key) => record[key]).find(Array.isArray)
    if (Array.isArray(nested)) rows = nested
    else {
      const text = firstText(record, ['content', 'text', 'message'])
      rows = text ? text.split('\n') : [record]
    }
  } else rows = String(data ?? '').split('\n')
  return rows.map(normalizeLogEntry).filter((line) => line.length > 0)
}
