type CellularDevice = Record<string, any>

export type CellularRegistrationState = 'registered' | 'searching' | 'denied' | 'unregistered' | 'unknown'

function modemOf(device: CellularDevice) {
  return (device?.modem || {}) as CellularDevice
}

function firstText(...values: unknown[]) {
  for (const value of values) {
    const text = String(value ?? '').trim()
    if (text) return text
  }
  return ''
}

export function cellularSignal(device: CellularDevice) {
  const modem = modemOf(device)
  const value = Number(modem.signal_dbm ?? device?.signal_dbm ?? -999)
  return Number.isFinite(value) && value !== 0 && value !== -999 ? value : undefined
}

function packetServiceEvidence(device: CellularDevice) {
  const modem = modemOf(device)
  const operator = firstText(modem.operator, device?.operator)
  const mode = firstText(modem.network_mode, device?.network_mode)
  const hasOperator = Boolean(operator && !/^(unknown|未知|—|-)$/.test(operator.toLowerCase()))
  const hasMode = Boolean(mode && !/^(unknown|未知|未驻网|—|-)$/.test(mode.toLowerCase()))
  return hasOperator && hasMode && cellularSignal(device) !== undefined
}

export function cellularRegistration(device: CellularDevice): {
  state: CellularRegistrationState
  label: string
  inferredPacketRegistration: boolean
} {
  const modem = modemOf(device)
  const rawStatus = modem.reg_status ?? modem.registration_status ?? device?.reg_status ?? device?.registration_status
  const status = Number(rawStatus)
  const text = firstText(modem.reg_status_text, modem.registration_state_label, device?.registration_state_label)
  const normalized = text.toLowerCase()
  const explicitlyUnregistered = text.includes('未注册') || /\b(not\s+registered|unregistered)\b/i.test(text)

  if ([1, 5].includes(status) || text.includes('已注册') || (!explicitlyUnregistered && /\b(registered|home|roaming)\b/i.test(text))) {
    return { state: 'registered', label: text || '已注册', inferredPacketRegistration: false }
  }

  // Some LTE modems expose circuit-switched CREG as "searching" while their
  // packet domain (CGREG/CEREG) is already registered. The upstream overview
  // currently returns only CREG, so use the same packet-service evidence as
  // the original interface: operator, radio mode and a usable signal.
  if (packetServiceEvidence(device)) {
    return { state: 'registered', label: '已注册（数据网络）', inferredPacketRegistration: true }
  }

  if (status === 2 || text.includes('搜索') || normalized.includes('search')) {
    return { state: 'searching', label: text || '搜索中', inferredPacketRegistration: false }
  }
  if (status === 3 || text.includes('拒绝') || normalized.includes('denied')) {
    return { state: 'denied', label: text || '注册被拒绝', inferredPacketRegistration: false }
  }
  if ([0, 4].includes(status) || explicitlyUnregistered || normalized.includes('not registered')) {
    return { state: 'unregistered', label: text || '未注册', inferredPacketRegistration: false }
  }
  return { state: 'unknown', label: text || '未知', inferredPacketRegistration: false }
}

export function cellularRegistered(device: CellularDevice) {
  return cellularRegistration(device).state === 'registered'
}
