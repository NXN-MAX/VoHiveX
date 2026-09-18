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
  network_enabled?: boolean
  local_phone?: string
  active_esim_profile_name?: string
  backend_mode?: string
  modem?: {
    imei?: string
    imsi?: string
    iccid?: string
    firmware?: string
    operator?: string
    network_mode?: string
    network_duplex?: string
    signal_dbm?: number
    signal_rsrp?: number
    signal_rsrq?: number
    signal_sinr?: number
    nr5g_signal_sinr?: number
    radio_band?: string
    radio_channel?: number | string
    reg_status_text?: string
    operating_mode?: number | string
    [key: string]: unknown
  }
  vowifi_runtime?: {
    sim_ready?: boolean
    access_ready?: boolean
    tunnel_ready?: boolean
    ims_ready?: boolean
    sms_ready?: boolean
    dataplane_mode?: string
    last_reason?: string
    last_error_class?: string
    [key: string]: unknown
  }
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
  last_timestamp?: number | string
  last_sms_id?: number
  unread?: number
  unread_count?: number
  is_unread?: boolean
  read?: boolean
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
  is_outgoing?: boolean
  type?: number | string
  read?: boolean
  status?: number | string
  delivery_status?: number | string
  send_status?: number | string
  delivery_state?: number | string
  [key: string]: unknown
}
