export function formatTemperature(value, unit = 'C') {
  if (value == null) return '—'
  return `${value.toFixed(1)} °${unit}`
}

export function formatHumidity(value) {
  if (value == null) return '—'
  return `${value.toFixed(1)} %`
}

export function formatTimestamp(ts) {
  if (!ts) return '—'
  return new Date(ts).toLocaleString()
}

export function decodeToken(token) {
  try {
    const payload = token.split('.')[1]
    return JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')))
  } catch {
    return null
  }
}

export function relativeTime(ts) {
  if (!ts) return '—'
  const diff = Math.max(0, Math.floor((Date.now() - new Date(ts).getTime()) / 1000))
  if (diff < 60) return `преди ${diff} сек`
  const mins = Math.floor(diff / 60)
  if (mins < 60) return `преди ${mins} мин`
  return `преди ${Math.floor(mins / 60)} ч`
}
