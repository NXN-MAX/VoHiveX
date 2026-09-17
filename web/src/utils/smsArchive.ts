const fields = ['device_id', 'device_name', 'local_phone', 'imsi', 'peer', 'message_id', 'direction', 'sender', 'content', 'timestamp', 'status'] as const
const aliases: Record<string, string[]> = {
  device_id: ['device_id', 'device', '设备ID'], device_name: ['device_name', '设备名称'], local_phone: ['local_phone', 'device_phone', '本机号码', '设备号码'],
  imsi: ['imsi'], peer: ['peer', 'contact', 'phone', '联系人', '对方号码'], message_id: ['message_id', 'id', '短信ID'],
  direction: ['direction', 'type', '方向'], sender: ['sender', '发送方'], content: ['content', 'message', 'text', '短信内容', '内容'], timestamp: ['timestamp', 'time', 'date', '时间'], status: ['status', '状态'],
}
const clean = (value: unknown) => String(value ?? '').replace(/^\uFEFF/, '').trim()
function pick(row: Record<string, unknown>, key: string) { for (const name of aliases[key]) if (row[name] !== undefined) return row[name]; return '' }
function direction(value: unknown) { return ['2', 'sent', 'outgoing', 'send', '已发送', '发送'].includes(clean(value).toLowerCase()) ? 2 : 1 }
function status(value: unknown) { const text = clean(value).toLowerCase(); if (!text) return null; if (['success', 'sent', 'delivered', '成功', '已发送'].includes(text)) return 2; if (['failed', 'error', '失败'].includes(text)) return 3; const number = Number(text); return Number.isInteger(number) && number >= 0 && number <= 3 ? number : null }

export function normalizeMessages(rows: Record<string, unknown>[]) {
  const output: Record<string, unknown>[] = []
  for (const source of rows) {
    const peer = clean(pick(source, 'peer')); const content = String(pick(source, 'content') ?? ''); const timestamp = clean(pick(source, 'timestamp'))
    if (!peer || !content || !timestamp || !Number.isFinite(Date.parse(timestamp))) continue
    output.push({ device_id: clean(pick(source, 'device_id')), device_name: clean(pick(source, 'device_name')), local_phone: clean(pick(source, 'local_phone')), imsi: clean(pick(source, 'imsi')), peer, message_id: clean(pick(source, 'message_id')), type: direction(pick(source, 'direction')), sender: clean(pick(source, 'sender')), content, timestamp: new Date(timestamp).toISOString(), status: status(pick(source, 'status')) })
  }
  if (!output.length) throw new Error('文件中没有可识别的短信记录')
  if (output.length > 20_000) throw new Error('一次最多导入 20000 条短信')
  return output
}

function parseDelimited(text: string, delimiter: string) {
  const rows: string[][] = []; let row: string[] = []; let field = ''; let quoted = false; const source = text.replace(/^\uFEFF/, '')
  for (let index = 0; index < source.length; index++) { const char = source[index]; if (quoted) { if (char === '"' && source[index + 1] === '"') { field += '"'; index++ } else if (char === '"') quoted = false; else field += char; continue } if (char === '"') quoted = true; else if (char === delimiter) { row.push(field); field = '' } else if (char === '\n') { row.push(field); rows.push(row); row = []; field = '' } else if (char !== '\r') field += char }
  row.push(field); if (row.some(Boolean)) rows.push(row); if (rows.length < 2) throw new Error('文件中没有短信数据')
  const headers = rows.shift()!.map(clean)
  return normalizeMessages(rows.filter((values) => values.some(Boolean)).map((values) => Object.fromEntries(headers.map((header, index) => [header, values[index] ?? '']))))
}
const escapeXml = (value: unknown) => String(value ?? '').replace(/[<>&"']/g, (char) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;', '"': '&quot;', "'": '&apos;' })[char]!)
const csvCell = (value: unknown) => { const text = String(value ?? ''); return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text }
function fieldValue(row: Record<string, any>, field: string) { if (field === 'direction') return Number(row.type) === 2 || row.outgoing ? 'sent' : 'received'; return row[field] ?? (field === 'content' ? row.text : field === 'message_id' ? row.id : '') }

export function serializeMessages(rows: Record<string, any>[], format: string) {
  if (!rows.length) throw new Error('没有可导出的短信')
  const normalized = rows.map((row) => Object.fromEntries(fields.map((field) => [field, fieldValue(row, field)])))
  if (format === 'csv') return '\uFEFF' + [fields.join(','), ...normalized.map((row) => fields.map((field) => csvCell(row[field])).join(','))].join('\r\n')
  if (format === 'txt') return [fields.join('\t'), ...normalized.map((row) => fields.map((field) => String(row[field] ?? '').replace(/[\t\r\n]+/g, ' ')).join('\t'))].join('\n')
  if (format === 'html') return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>VoHiveX 短信归档</title></head><body><table><thead><tr>${fields.map((field) => `<th>${field}</th>`).join('')}</tr></thead><tbody>${normalized.map((row) => `<tr>${fields.map((field) => `<td>${escapeXml(row[field])}</td>`).join('')}</tr>`).join('')}</tbody></table></body></html>`
  if (format === 'xml') return `<?xml version="1.0" encoding="UTF-8"?>\n<smsArchive version="1">\n${normalized.map((row) => `  <message>${fields.map((field) => `<${field}>${escapeXml(row[field])}</${field}>`).join('')}</message>`).join('\n')}\n</smsArchive>`
  throw new Error('不支持的导出格式')
}

export function parseArchive(text: string, format: string) {
  if (format === 'csv') return parseDelimited(text, ',')
  if (format === 'txt') return parseDelimited(text, '\t')
  if (!['html', 'xml'].includes(format)) throw new Error('仅支持 CSV、TXT、HTML、XML')
  const document = new DOMParser().parseFromString(text, format === 'html' ? 'text/html' : 'application/xml')
  if (document.querySelector('parsererror')) throw new Error('文件结构无效')
  if (format === 'html') { const table = document.querySelector('table'); if (!table) throw new Error('HTML 中没有短信表格'); const headers = [...table.querySelectorAll('thead th')].map((cell) => clean(cell.textContent)); return normalizeMessages([...table.querySelectorAll('tbody tr')].map((tr) => Object.fromEntries([...tr.querySelectorAll('td')].map((cell, index) => [headers[index] || '', cell.textContent || ''])))) }
  return normalizeMessages([...document.querySelectorAll('message')].map((node) => Object.fromEntries(fields.map((field) => [field, node.querySelector(field)?.textContent || '']))))
}
