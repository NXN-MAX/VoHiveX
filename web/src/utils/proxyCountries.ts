import operatorRows from '@/data/mcc-mnc.json'

export interface ProxyCountry {
  country_code: string
  country_name: string
  mccs: string[]
}
type OperatorRow = {
  plmn?: string
  iso?: string
  country?: string
}

function normalizedCode(value: unknown) {
  const code = String(value || '').trim().toUpperCase()
  return /^[A-Z]{2}$/.test(code) ? code : ''
}

function normalizedMccs(value: unknown) {
  if (!Array.isArray(value)) return []
  return [...new Set(value.map((item) => String(item || '').trim()).filter((item) => /^\d{3}$/.test(item)))].sort()
}

function regionName(code: string, fallback = '') {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' }).of(code) || fallback || code
  } catch {
    return fallback || code
  }
}

const bundledCountries: ProxyCountry[] = (() => {
  const grouped = new Map<string, { fallbackName: string; mccs: Set<string> }>()

  for (const row of operatorRows as OperatorRow[]) {
    const code = normalizedCode(row.iso)
    const plmn = String(row.plmn || '').trim()
    if (!code || !/^\d{5,6}$/.test(plmn)) continue

    const current = grouped.get(code) || { fallbackName: '', mccs: new Set<string>() }
    if (!current.fallbackName && row.country) current.fallbackName = String(row.country).trim()
    current.mccs.add(plmn.slice(0, 3))
    grouped.set(code, current)
  }

  return [...grouped.entries()]
    .map(([country_code, value]) => ({
      country_code,
      country_name: regionName(country_code, value.fallbackName),
      mccs: [...value.mccs].sort(),
    }))
    .sort((left, right) => left.country_name.localeCompare(right.country_name, 'en'))
})()

export function mergeProxyCountries(apiCountries: unknown): ProxyCountry[] {
  const merged = new Map(bundledCountries.map((country) => [country.country_code, { ...country, mccs: [...country.mccs] }]))

  if (Array.isArray(apiCountries)) {
    for (const item of apiCountries) {
      if (!item || typeof item !== 'object') continue
      const row = item as Record<string, unknown>
      const code = normalizedCode(row.country_code || row.code)
      if (!code) continue

      const existing = merged.get(code)
      const mccs = normalizedMccs(row.mccs)
      merged.set(code, {
        country_code: code,
        country_name: String(row.country_name || row.name || existing?.country_name || regionName(code)).trim(),
        mccs: [...new Set([...(existing?.mccs || []), ...mccs])].sort(),
      })
    }
  }

  return [...merged.values()].sort((left, right) => left.country_name.localeCompare(right.country_name, 'en'))
}
