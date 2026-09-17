export interface Device {
  id: string
  name?: string
  running?: boolean
  healthy?: boolean
  operator?: string
  network_mode?: string
  network_duplex?: string
  signal_dbm?: number
  vowifi_active?: boolean
  vowifi_enabled?: boolean
  network_connected?: boolean
  public_ip?: string
  public_ipv6?: string
  private_ip?: string
  private_ipv6?: string
  interface?: string
  iface?: string
  imei?: string
  imsi?: string
  iccid?: string
  msisdn?: string
  flight_mode?: boolean
  operating_mode?: string
  [key: string]: unknown
}

export interface ManagedProxyNode {
  id: string
  source_id?: string
  name: string
  type?: string
  server?: string
  port?: number
  delay_ms?: number
  enabled?: boolean
  [key: string]: unknown
}

export interface ManagedProxySource {
  id: string
  name: string
  kind?: string
  updated_at?: number | string
  node_count?: number
  built_in?: boolean
  [key: string]: unknown
}

export interface ScheduleTask {
  id: number
  version: number
  name: string
  device_id: string
  device_name?: string
  phone: string
  message: string
  mode: 'once' | 'interval'
  first_run: number
  interval_seconds?: number
  enabled?: boolean
  status?: string
  next_run?: number
  last_run?: number
  run_count?: number
  last_status?: string
  last_detail?: string
  [key: string]: unknown
}

export interface SmsContact {
  peer: string
  device_id?: string
  device_name?: string
  imsi?: string
  local_phone?: string
  last_content?: string
  last_ts?: number | string
  last_sms_id?: number
  unread?: number
  [key: string]: unknown
}

export interface SmsMessage {
  id: number
  peer: string
  device_id?: string
  device_name?: string
  content?: string
  text?: string
  timestamp?: number | string
  direction?: string
  outgoing?: boolean
  read?: boolean
  status?: string
  [key: string]: unknown
}
