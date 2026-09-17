import operatorRows from '@/data/mcc-mnc.json'

type OperatorRow = { plmn: string; iso?: string; network?: string; country?: string }
type OperatorDisplay = { display: string; flag: string; name: string; plmn: string }

const operatorByPlmn = new Map((operatorRows as OperatorRow[]).map((row) => [row.plmn, row]))
const operatorByMcc = new Map<string, OperatorRow>()
for (const row of operatorRows as OperatorRow[]) {
  const mcc = row.plmn.slice(0, 3)
  if (!operatorByMcc.has(mcc)) operatorByMcc.set(mcc, row)
}

function text(value: unknown) { return String(value ?? '').trim() }

function flagFromIso(value: unknown) {
  const iso = text(value).toUpperCase()
  if (!/^[A-Z]{2}$/.test(iso)) return ''
  return String.fromCodePoint(...[...iso].map((letter) => 127462 + letter.charCodeAt(0) - 65))
}

function pnnName(value: any) {
  return text(value?.full_name) || text(value?.short_name)
}

function matchesPlmn(pattern: unknown, plmn: string) {
  const value = text(pattern).toLowerCase()
  if (!value || !plmn) return false
  if (value === plmn) return true
  if (!value.includes('x')) return value.length < plmn.length && plmn.startsWith(value)
  if (value.length !== plmn.length) return false
  return [...value].every((character, index) => character === 'x' || character === plmn[index])
}

function operatorFromPnn(modem: any, plmn: string) {
  const pnn = Array.isArray(modem?.pnn) ? modem.pnn : []
  const opl = Array.isArray(modem?.opl) ? modem.opl : []
  for (const rule of opl) {
    if (!matchesPlmn(rule?.plmn, plmn)) continue
    const record = Number(rule?.pnn_record || 0)
    const name = pnnName(pnn.find((row: any) => Number(row?.record) === record))
    if (name) return name
  }
  for (const row of pnn) {
    const name = pnnName(row)
    if (name) return name
  }
  return ''
}

function findOperator(mcc: string, mnc: string) {
  const candidates = [...new Set([`${mcc}${mnc}`, `${mcc}${mnc.padStart(2, '0')}`, `${mcc}${mnc.padStart(3, '0')}`])]
  for (const candidate of candidates) {
    const row = operatorByPlmn.get(candidate)
    if (row) return { row, plmn: candidate }
  }
  return { row: operatorByMcc.get(mcc), plmn: `${mcc}${mnc}` }
}

export function formatSimOperator(sources: unknown[], fallback = ''): OperatorDisplay {
  const combined: Record<string, any> = {}
  const modem: Record<string, any> = {}
  for (const source of sources) {
    if (!source || typeof source !== 'object') continue
    Object.assign(combined, source)
    const nested = (source as any).modem
    if (nested && typeof nested === 'object') Object.assign(modem, nested)
  }
  Object.assign(modem, Object.fromEntries(Object.entries(combined).filter(([key]) => !(key in modem))))

  const mcc = text(modem.native_mcc || modem.home_mcc || modem.sim_mcc)
  const mnc = text(modem.native_mnc || modem.home_mnc || modem.sim_mnc)
  const match = mcc ? findOperator(mcc, mnc) : { row: undefined, plmn: '' }
  const plmn = mcc && mnc ? match.plmn : ''
  const name = text(modem.native_spn)
    || operatorFromPnn(modem, plmn)
    || text(match.row?.network)
    || text(fallback)
  const iso = text(modem.native_country_code || modem.sim_country_code || modem.country_code || match.row?.iso)
  const flag = flagFromIso(iso)
  const base = name || text(match.row?.country) || plmn || '—'
  return {
    display: plmn && base !== plmn ? `${base} (${plmn})` : base,
    flag,
    name: base,
    plmn,
  }
}
