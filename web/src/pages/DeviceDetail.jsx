import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  LineChart, Line, BarChart, Bar,
  XAxis, YAxis, CartesianGrid, Tooltip,
  ResponsiveContainer, Legend,
} from 'recharts'
import { useAuth } from '../context/AuthContext'
import {
  listDevices, getCurrentReading, getDeviceStatus, getHistory, getMetricHistory,
  getSettings, saveSettings, switchMode, setLight, listAlertRules, createAlertRule, deleteAlertRule,
  getCompressorCycles, getErrors, getDeviceTypes,
  setDeviceType as apiSetDeviceType,
  updateDeviceName, getDeviceLogs,
} from '../api/index'
import {
  formatTemperature, formatHumidity, formatTimestamp,
  decodeToken, relativeTime,
} from '../utils/index'
import './DeviceDetail.css'

const SYSTEM_STATE_LABELS = ['Нормален', 'Предупреждение', 'Грешка', 'Безопасен режим', 'Резервен']
const SYSTEM_STATE_CLASSES = ['normal', 'warning', 'error', 'safe-mode', 'fallback']
const TEMP_THRESHOLD = 8.0

function formatChartTime(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function formatChartDate(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  return `${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')}`
}

function hasActiveError(errors) {
  return Array.isArray(errors) && errors.some((e) => e.active)
}

const SEVERITY_LABELS  = ['Инфо', 'Предупреждение', 'Грешка']
const SEVERITY_CLASSES = ['sev-info', 'sev-warning', 'sev-error']

// ── Tabs ──────────────────────────────────────────────────
function Tabs({ active, onChange }) {
  const tabs = [
    { key: 'history',     label: 'История' },
    { key: 'settings',    label: 'Настройки' },
    { key: 'alerts',      label: 'Алерти' },
    { key: 'modes',       label: 'Режими' },
    { key: 'diagnostics', label: 'Диагностика' },
    { key: 'logs',        label: 'Логове' },
  ]
  return (
    <div className="dd-tabs">
      {tabs.map((t) => (
        <button
          key={t.key}
          className={`dd-tab${active === t.key ? ' dd-tab-active' : ''}`}
          onClick={() => onChange(t.key)}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}

// ── Chart tooltip ─────────────────────────────────────────
function ChartTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null
  return (
    <div className="chart-tooltip">
      <p className="chart-tooltip-time">{label}</p>
      {payload.map((p) => (
        <p key={p.dataKey} style={{ color: p.color }}>
          {p.name}: {p.value != null ? p.value.toFixed(1) : '—'}
          {p.dataKey === 'temperature' ? ' °C' : ' %'}
        </p>
      ))}
    </div>
  )
}

const DAYS_OPTIONS = [1, 3, 7, 14, 31]

// ── Tab: История ──────────────────────────────────────────
function TabHistory({ current, status, history, days, setDays }) {
  const compressorOn = status?.device_states?.compressor ?? false
  const fanOn = status?.device_states?.extra_fan ?? false
  const lightOn = status?.device_states?.light ?? false
  const heatingOn = status?.device_states?.heating ?? false
  const systemState = status?.system_status?.state ?? null
  const stateLabel = SYSTEM_STATE_LABELS[systemState] ?? 'Неизвестно'
  const stateClass = SYSTEM_STATE_CLASSES[systemState] ?? ''
  const tempHigh = current?.temperature != null && current.temperature > TEMP_THRESHOLD

  const chartData = [...(history ?? [])].reverse().map((r) => ({
    time: formatChartTime(r.timestamp),
    temperature: r.temperature ?? null,
    humidity: r.humidity ?? null,
  }))

  const hideXLabels = days >= 7

  return (
    <div className="dd-tab-content">
      <div className="dd-range-group">
        {DAYS_OPTIONS.map((d) => (
          <button
            key={d}
            className={`dd-range-btn${days === d ? ' dd-range-btn-active' : ''}`}
            onClick={() => setDays(d)}
          >
            {d}д
          </button>
        ))}
      </div>
      <div className="dd-chart-wrap">
        <ResponsiveContainer width="100%" height={300}>
          <LineChart data={chartData} margin={{ top: 8, right: 24, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#2a2d3a" />
            <XAxis
              dataKey="time"
              tick={hideXLabels ? false : { fill: '#64748b', fontSize: 11 }}
              tickLine={false}
            />
            <YAxis
              yAxisId="temp"
              orientation="left"
              tick={{ fill: '#64748b', fontSize: 11 }}
              tickLine={false}
              axisLine={false}
              unit="°"
            />
            <YAxis
              yAxisId="hum"
              orientation="right"
              tick={{ fill: '#64748b', fontSize: 11 }}
              tickLine={false}
              axisLine={false}
              unit="%"
            />
            <Tooltip content={<ChartTooltip />} />
            <Legend
              wrapperStyle={{ fontSize: '12px', color: '#94a3b8', paddingTop: '8px' }}
            />
            <Line
              yAxisId="temp"
              type="monotone"
              dataKey="temperature"
              name="Температура"
              stroke="#ef4444"
              dot={false}
              strokeWidth={2}
              connectNulls
            />
            <Line
              yAxisId="hum"
              type="monotone"
              dataKey="humidity"
              name="Влажност"
              stroke="#3b82f6"
              dot={false}
              strokeWidth={2}
              connectNulls
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="dd-stats-row">
        <div className="dd-stat">
          <span className="dd-stat-label">Температура</span>
          <span className={`dd-stat-value${tempHigh ? ' temp-high' : ''}`}>
            {formatTemperature(current?.temperature)}
          </span>
        </div>
        <div className="dd-stat">
          <span className="dd-stat-label">Влажност</span>
          <span className="dd-stat-value hum-value">
            {formatHumidity(current?.humidity)}
          </span>
        </div>
        <div className="dd-stat">
          <span className="dd-stat-label">Компресор</span>
          <span className={`relay-badge ${compressorOn ? 'relay-on' : 'relay-off'}`}>
            {compressorOn ? 'ON' : 'OFF'}
          </span>
        </div>
        <div className="dd-stat">
          <span className="dd-stat-label">Вентилатор</span>
          <span className={`relay-badge ${fanOn ? 'relay-on' : 'relay-off'}`}>
            {fanOn ? 'ON' : 'OFF'}
          </span>
        </div>
        <div className="dd-stat">
          <span className="dd-stat-label">Осветление</span>
          <span className={`relay-badge ${lightOn ? 'relay-on' : 'relay-off'}`}>
            {lightOn ? 'ON' : 'OFF'}
          </span>
        </div>
        <div className="dd-stat">
          <span className="dd-stat-label">Нагревател</span>
          <span className={`relay-badge ${heatingOn ? 'relay-on' : 'relay-off'}`}>
            {heatingOn ? 'ON' : 'OFF'}
          </span>
        </div>
        <div className="dd-stat">
          <span className="dd-stat-label">Статус</span>
          <span className={`state-badge state-${stateClass}`}>{stateLabel}</span>
        </div>
      </div>

      <p className="dd-updated">
        Обновено: {relativeTime(current?.timestamp)}
      </p>
    </div>
  )
}

// ── Tab: Настройки ────────────────────────────────────────
function TabSettings({ settings, tenantId, deviceId, deviceTypes, deviceTypeId, setDeviceTypeId, isAdmin }) {
  const initialForm = useCallback(() => {
    const temp = settings?.temp ?? {}
    const hum  = settings?.humidity ?? {}
    const fan  = settings?.fan ?? {}
    return {
      temp_target:       temp.target        ?? 4,
      temp_offset:       temp.offset        ?? 0,
      hum_target:        hum.target         ?? 80,
      hum_offset:        hum.offset         ?? 0,
      fan_speed:         fan.speed          ?? 50,
      mixing_enabled:    fan.mixing_enabled ?? true,
      mixing_interval:   Math.round((fan.mixing_interval_s ?? 3600) / 60),
      mixing_duration:   Math.round((fan.mixing_duration_s ?? 300) / 60),
    }
  }, [settings])

  const [form, setForm] = useState(initialForm)
  const [saving, setSaving] = useState(false)
  const [saveMsg, setSaveMsg] = useState(null) // { type: 'ok'|'err', text }
  const successTimer = useRef(null)

  // Device type local state
  const [selectedType, setSelectedType] = useState(deviceTypeId ?? '')
  const [typeSaving, setTypeSaving] = useState(false)
  const [typeMsg, setTypeMsg] = useState(null)
  const typeMsgTimer = useRef(null)

  // Light local state (0=manual, 1=auto)
  const [lightMode, setLightMode] = useState(settings?.light?.mode ?? 0)
  const [lightState, setLightState] = useState(settings?.light?.state ?? false)
  const [lightSaving, setLightSaving] = useState(false)
  const [lightMsg, setLightMsg] = useState(null)
  const lightMsgTimer = useRef(null)

  // Sync light state when settings loads (initial value is null before fetch completes)
  useEffect(() => {
    if (settings?.light != null) {
      setLightMode(settings.light.mode ?? 0)
      setLightState(settings.light.state ?? false)
    }
  }, [settings])

  async function handleLightMode(newMode) {
    if (lightSaving) return
    setLightSaving(true)
    setLightMsg(null)
    try {
      await setLight(tenantId, deviceId, { mode: newMode })
      setLightMode(newMode === 'manual' ? 0 : 1)
      setLightMsg({ type: 'ok', text: 'Режимът е запазен' })
      clearTimeout(lightMsgTimer.current)
      lightMsgTimer.current = setTimeout(() => setLightMsg(null), 3000)
    } catch (err) {
      console.error('handleLightMode error:', err.response?.status, err.response?.data)
      setLightMsg({ type: 'err', text: err.response?.data || 'Грешка' })
    } finally {
      setLightSaving(false)
    }
  }

  async function handleLightToggle() {
    if (lightSaving) return
    const newState = !lightState
    setLightSaving(true)
    setLightMsg(null)
    try {
      await setLight(tenantId, deviceId, { state: newState })
      setLightState(newState)
      setLightMsg({ type: 'ok', text: newState ? 'Осветлението е включено' : 'Осветлението е изключено' })
      clearTimeout(lightMsgTimer.current)
      lightMsgTimer.current = setTimeout(() => setLightMsg(null), 3000)
    } catch (err) {
      console.error('handleLightToggle error:', err.response?.status, err.response?.data)
      setLightMsg({ type: 'err', text: err.response?.data || 'Грешка' })
    } finally {
      setLightSaving(false)
    }
  }

  async function handleSaveType() {
    if (!selectedType) return
    setTypeSaving(true)
    setTypeMsg(null)
    try {
      await apiSetDeviceType(tenantId, deviceId, selectedType)
      setDeviceTypeId(selectedType)
      setTypeMsg({ type: 'ok', text: 'Типът е запазен' })
      clearTimeout(typeMsgTimer.current)
      typeMsgTimer.current = setTimeout(() => setTypeMsg(null), 3000)
    } catch (err) {
      setTypeMsg({ type: 'err', text: err.response?.data?.error || 'Грешка при запазване' })
    } finally {
      setTypeSaving(false)
    }
  }

  // Re-sync form when settings load/change
  useEffect(() => {
    setForm(initialForm())
  }, [initialForm])

  function setField(key, value) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setSaveMsg(null)
    try {
      await saveSettings(tenantId, deviceId, {
        temp:     { target: parseFloat(form.temp_target), offset: parseFloat(form.temp_offset) },
        humidity: { target: parseFloat(form.hum_target),  offset: parseFloat(form.hum_offset) },
        fan: {
          speed:            parseInt(form.fan_speed, 10),
          mixing_enabled:   form.mixing_enabled,
          mixing_interval:  parseInt(form.mixing_interval, 10) * 60,
          mixing_duration:  parseInt(form.mixing_duration, 10) * 60,
        },
      })
      setSaveMsg({ type: 'ok', text: 'Настройките са запазени' })
      clearTimeout(successTimer.current)
      successTimer.current = setTimeout(() => setSaveMsg(null), 3000)
    } catch (err) {
      setSaveMsg({ type: 'err', text: err.response?.data?.error || 'Грешка при запазване' })
    } finally {
      setSaving(false)
    }
  }

  if (!settings) return <p className="dd-empty">Няма налични настройки.</p>

  const currentTypeName = deviceTypes?.find((t) => t.id === (selectedType || deviceTypeId))?.display_name

  return (
    <div className="dd-tab-content">

      {/* ── Device type selector (admin only) ── */}
      {isAdmin && deviceTypes?.length > 0 && (
        <div className="dd-device-type-section">
          <h3 className="dd-form-title">Тип устройство</h3>
          <div className="dd-settings-list">
            <div className="dd-settings-row">
              <label className="dd-settings-label" htmlFor="device_type_select">Тип</label>
              <select
                id="device_type_select"
                className="dd-settings-input dd-select"
                value={selectedType}
                onChange={(e) => setSelectedType(e.target.value)}
              >
                <option value="">Не е зададен</option>
                {deviceTypes.map((dt) => (
                  <option key={dt.id} value={dt.id}>{dt.display_name}</option>
                ))}
              </select>
            </div>
            {!isAdmin && (
              <div className="dd-settings-row">
                <span className="dd-settings-label">Текущ тип</span>
                <span className="dd-settings-value">{currentTypeName ?? 'Не е зadadен'}</span>
              </div>
            )}
          </div>
          {typeMsg && (
            <p className={`dd-save-msg ${typeMsg.type === 'ok' ? 'dd-save-ok' : 'dd-save-err'}`}>
              {typeMsg.text}
            </p>
          )}
          <button className="dd-save-btn" type="button" disabled={typeSaving || !selectedType} onClick={handleSaveType}>
            {typeSaving ? 'Запазване...' : 'Запази тип'}
          </button>
        </div>
      )}

      {/* ── Light control (admin only) ── */}
      {isAdmin && (
        <div className="dd-device-type-section">
          <h3 className="dd-form-title">Осветление</h3>
          <div className="dd-settings-list">
            <div className="dd-settings-row">
              <span className="dd-settings-label">Режим</span>
              <div className="dd-range-group" style={{ marginBottom: 0 }}>
                <button
                  type="button"
                  className={`dd-range-btn${lightMode === 0 ? ' dd-range-btn-active' : ''}`}
                  disabled={lightSaving}
                  onClick={() => handleLightMode('manual')}
                >
                  Ръчен
                </button>
                <button
                  type="button"
                  className={`dd-range-btn${lightMode === 1 ? ' dd-range-btn-active' : ''}`}
                  disabled={lightSaving}
                  onClick={() => handleLightMode('auto')}
                >
                  Автоматичен
                </button>
              </div>
            </div>
            {lightMode === 0 && (
              <div className="dd-settings-row">
                <span className="dd-settings-label">Светлина</span>
                <button
                  type="button"
                  className={`dd-light-toggle${lightState ? ' dd-light-toggle-on' : ' dd-light-toggle-off'}`}
                  disabled={lightSaving}
                  onClick={handleLightToggle}
                >
                  {lightState ? 'Включено' : 'Изключено'}
                </button>
              </div>
            )}
          </div>
          {lightMsg && (
            <p className={`dd-save-msg ${lightMsg.type === 'ok' ? 'dd-save-ok' : 'dd-save-err'}`}>
              {lightMsg.text}
            </p>
          )}
        </div>
      )}

      <form className="dd-settings-form" onSubmit={handleSave}>

        {/* ── Temperature card ── */}
        <div className="dd-settings-card dd-settings-card--temp">
          <div className="dd-settings-card-title">🌡️ Температура</div>
          <div className="dd-settings-grid">
            <label className="dd-settings-label" htmlFor="temp_target">Target (°C)</label>
            <input
              id="temp_target"
              className="dd-settings-input"
              type="number" step="0.5" min="-30" max="30"
              value={form.temp_target}
              onChange={(e) => setField('temp_target', e.target.value)}
            />
            <label className="dd-settings-label" htmlFor="temp_offset">Офсет (°C)</label>
            <input
              id="temp_offset"
              className="dd-settings-input"
              type="number" step="0.1" min="-10" max="10"
              value={form.temp_offset}
              onChange={(e) => setField('temp_offset', e.target.value)}
            />
          </div>
        </div>

        {/* ── Humidity card ── */}
        <div className="dd-settings-card dd-settings-card--hum">
          <div className="dd-settings-card-title">💧 Влажност</div>
          <div className="dd-settings-grid">
            <label className="dd-settings-label" htmlFor="hum_target">Target (%)</label>
            <input
              id="hum_target"
              className="dd-settings-input"
              type="number" step="1" min="0" max="100"
              value={form.hum_target}
              onChange={(e) => setField('hum_target', e.target.value)}
            />
            <label className="dd-settings-label" htmlFor="hum_offset">Офсет (%)</label>
            <input
              id="hum_offset"
              className="dd-settings-input"
              type="number" step="0.1" min="-10" max="10"
              value={form.hum_offset}
              onChange={(e) => setField('hum_offset', e.target.value)}
            />
          </div>
        </div>

        {/* ── Fan card ── */}
        <div className="dd-settings-card dd-settings-card--fan">
          <div className="dd-settings-card-title">🌀 Вентилатор</div>
          <div className="dd-settings-grid">
            <label className="dd-settings-label" htmlFor="fan_speed">Скорост (%)</label>
            <input
              id="fan_speed"
              className="dd-settings-input"
              type="number" step="1" min="0" max="100"
              value={form.fan_speed}
              onChange={(e) => setField('fan_speed', e.target.value)}
            />
            <label className="dd-settings-label" htmlFor="mixing_enabled">Миксиране</label>
            <label className="dd-toggle">
              <input
                id="mixing_enabled"
                type="checkbox"
                checked={form.mixing_enabled}
                onChange={(e) => setField('mixing_enabled', e.target.checked)}
              />
              <span className="dd-toggle-track">
                <span className="dd-toggle-thumb" />
              </span>
              <span className="dd-toggle-label">
                {form.mixing_enabled ? 'включено' : 'изключено'}
              </span>
            </label>
            {form.mixing_enabled && (
              <>
                <label className="dd-settings-label" htmlFor="mixing_interval">Интервал (мин)</label>
                <input
                  id="mixing_interval"
                  className="dd-settings-input"
                  type="number" step="1" min="1"
                  value={form.mixing_interval}
                  onChange={(e) => setField('mixing_interval', e.target.value)}
                />
                <label className="dd-settings-label" htmlFor="mixing_duration">Продължителност (мин)</label>
                <input
                  id="mixing_duration"
                  className="dd-settings-input"
                  type="number" step="1" min="1"
                  value={form.mixing_duration}
                  onChange={(e) => setField('mixing_duration', e.target.value)}
                />
              </>
            )}
          </div>
        </div>

        {saveMsg && (
          <p className={`dd-save-msg ${saveMsg.type === 'ok' ? 'dd-save-ok' : 'dd-save-err'}`}>
            {saveMsg.text}
          </p>
        )}

        <button className="dd-save-btn" type="submit" disabled={saving}>
          {saving ? 'Запазване...' : 'Запази'}
        </button>
      </form>
    </div>
  )
}

// ── Tab: Алерти ───────────────────────────────────────────
const OPERATOR_SENTENCE = { gt: 'по-висока от', lt: 'по-ниска от', gte: 'равна или по-висока от', lte: 'равна или по-ниска от' }
const METRIC_ICONS      = { temperature: '🌡️', humidity: '💧' }
const METRIC_NAMES      = { temperature: 'Температура', humidity: 'Влажност' }

const EMPTY_RULE = {
  metric: 'temperature',
  operator: 'gt',
  threshold: '',
  recipient: '',
  cooldown_minutes: 15,
  enabled: true,
}

function TabAlerts({ rules, setRules, tenantId, deviceId }) {
  const [form, setForm] = useState({ ...EMPTY_RULE })
  const [adding, setAdding]   = useState(false)
  const [addMsg, setAddMsg]   = useState(null)
  const addTimer = useRef(null)

  function setField(key, value) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  async function handleAdd(e) {
    e.preventDefault()
    setAdding(true)
    setAddMsg(null)
    try {
      await createAlertRule(tenantId, deviceId, {
        metric:           form.metric,
        operator:         form.operator,
        threshold:        parseFloat(form.threshold),
        channel:          'email',
        recipient:        form.recipient,
        enabled:          form.enabled,
        cooldown_minutes: parseInt(form.cooldown_minutes, 10),
      })
      const { data } = await listAlertRules(tenantId, deviceId)
      setRules(data ?? [])
      setForm({ ...EMPTY_RULE })
      setAddMsg({ type: 'ok', text: 'Известието е добавено' })
      clearTimeout(addTimer.current)
      addTimer.current = setTimeout(() => setAddMsg(null), 3000)
    } catch (err) {
      setAddMsg({ type: 'err', text: err.response?.data?.error || 'Грешка при добавяне' })
    } finally {
      setAdding(false)
    }
  }

  async function handleDelete(rule) {
    if (!window.confirm('Изтриване на правилото?')) return
    try {
      await deleteAlertRule(tenantId, deviceId, rule.id)
      setRules((prev) => prev.filter((r) => r.id !== rule.id))
    } catch (err) {
      alert(err.response?.data?.error || 'Грешка при изтриване')
    }
  }

  const unit = form.metric === 'temperature' ? '°C' : '%'

  return (
    <div className="dd-tab-content">

      {/* ── Create form ── */}
      <form className="dd-alert-form" onSubmit={handleAdd}>
        <div className="dd-settings-card">
          <div className="dd-settings-card-title">🔔 Ново известие</div>

          {/* Line 1: Изпрати имейл до [email] */}
          <div className="dd-alert-line">
            <span className="dd-alert-prose">Изпрати имейл до</span>
            <input
              id="al_recipient"
              className="dd-settings-input dd-alert-email-input"
              type="email" required placeholder="user@example.com"
              value={form.recipient}
              onChange={(e) => setField('recipient', e.target.value)}
            />
          </div>

          {/* Line 2: когато [метрика] е [оператор] */}
          <div className="dd-alert-line">
            <span className="dd-alert-prose">когато</span>
            <select
              id="al_metric"
              className="dd-settings-input dd-select dd-alert-select"
              value={form.metric}
              onChange={(e) => setField('metric', e.target.value)}
            >
              <option value="temperature">температурата</option>
              <option value="humidity">влажността</option>
            </select>
            <span className="dd-alert-prose">е</span>
            <select
              id="al_operator"
              className="dd-settings-input dd-select dd-alert-select"
              value={form.operator}
              onChange={(e) => setField('operator', e.target.value)}
            >
              <option value="gt">по-висока от</option>
              <option value="lt">по-ниска от</option>
              <option value="gte">равна или по-висока от</option>
              <option value="lte">равна или по-ниска от</option>
            </select>
          </div>

          {/* Line 3: [threshold] °C/% */}
          <div className="dd-alert-line">
            <input
              id="al_threshold"
              className="dd-settings-input dd-alert-threshold"
              type="number" step="0.1" required
              value={form.threshold}
              onChange={(e) => setField('threshold', e.target.value)}
            />
            <span className="dd-alert-unit">{unit}</span>
          </div>

          {/* Line 4: Не изпращай повторно в рамките на [n] минути */}
          <div className="dd-alert-line">
            <span className="dd-alert-prose">Не изпращай повторно в рамките на</span>
            <input
              id="al_cooldown"
              className="dd-settings-input dd-alert-number"
              type="number" step="1" min="1"
              value={form.cooldown_minutes}
              onChange={(e) => setField('cooldown_minutes', e.target.value)}
            />
            <span className="dd-alert-prose">минути</span>
          </div>

          {/* Line 5: toggle + submit */}
          <div className="dd-alert-line dd-alert-line--footer">
            <label className="dd-toggle">
              <input
                id="al_enabled"
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setField('enabled', e.target.checked)}
              />
              <span className="dd-toggle-track"><span className="dd-toggle-thumb" /></span>
              <span className="dd-toggle-label">Активно</span>
            </label>
            <button className="dd-save-btn dd-alert-add-btn" type="submit" disabled={adding}>
              {adding ? 'Добавяне...' : 'Добави известие'}
            </button>
          </div>
        </div>

        {addMsg && (
          <p className={`dd-save-msg ${addMsg.type === 'ok' ? 'dd-save-ok' : 'dd-save-err'}`}>
            {addMsg.text}
          </p>
        )}
      </form>

      {/* ── Rules list ── */}
      {rules.length === 0 ? (
        <p className="dd-empty">Няма настроени известия.</p>
      ) : (
        <ul className="dd-alert-list">
          {rules.map((rule) => (
            <li key={rule.id} className="dd-alert-item">
              <div className="dd-alert-item-top">
                <span className="dd-alert-condition">
                  {METRIC_ICONS[rule.metric]}{' '}
                  {METRIC_NAMES[rule.metric] ?? rule.metric}{' '}
                  {OPERATOR_SENTENCE[rule.operator] ?? rule.operator}{' '}
                  {rule.threshold}{rule.metric === 'temperature' ? '°C' : '%'}
                  {' → '}{rule.recipient}
                </span>
                <div className="dd-alert-item-actions">
                  <span className={`dd-alert-status ${rule.enabled ? 'alert-enabled' : 'alert-disabled'}`}>
                    {rule.enabled ? 'активен' : 'неактивен'}
                  </span>
                  <button className="dd-delete-btn" type="button" onClick={() => handleDelete(rule)}>
                    Изтрий
                  </button>
                </div>
              </div>
              <div className="dd-alert-item-bottom">
                <span className="dd-alert-cooldown">Cooldown: {rule.cooldown_minutes} мин</span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ── Режими ────────────────────────────────────────────────
const MODE_STRING_TO_INT = {
  normal:              0,
  heating:             1,
  beer_cooling:        2,
  room_temp:           3,
  product_meat_fish:   10,
  product_dairy:       11,
  product_ready_food:  12,
  product_vegetables:  13,
}

const MODES = [
  {
    id: 'basic',
    label: 'ОСНОВНИ РЕЖИМИ',
    modes: [
      { value: 0,  name: 'Режим сушилня',       desc: 'Стандартно охлаждане, когато външната температура е по-висока от зададената', fixed: null },
      { value: 1,  name: 'Режим на нагряване',  desc: 'Приоритетно нагряване за поддържане на температура над външната',            fixed: null },
      { value: 3,  name: 'Стайна температура',  desc: 'Поддържа комфортна стайна температура с нагряване и охлаждане',              fixed: '21°C ± 1°C' },
      { value: 2,  name: 'Охлаждане на бира',   desc: 'Прецизно поддържане на ниска температура за напитки',                        fixed: '7.5°C ± 1°C' },
    ],
  },
  {
    id: 'product',
    label: 'ПРОДУКТОВИ РЕЖИМИ',
    modes: [
      { value: 10, name: 'Месо и риба',          desc: 'Оптимална температура за съхранение на свежо месо и риба',               fixed: '1.5°C ± 1°C' },
      { value: 11, name: 'Млечни продукти',      desc: 'Подходящ за мляко, сирене, кисело мляко и млечни продукти',              fixed: '3.5°C ± 1°C' },
      { value: 12, name: 'Готови храни',         desc: 'За приготвени и готови за консумация храни',                              fixed: '5.5°C ± 1°C' },
      { value: 13, name: 'Плодове и зеленчуци', desc: 'Запазва свежестта на плодове и зеленчуци по-дълго време',                 fixed: '9°C ± 1°C' },
    ],
  },
]

const ALL_MODES_FLAT = MODES.flatMap((g) => g.modes)

function Toast({ msg }) {
  if (!msg) return null
  return (
    <div className={`dd-toast ${msg.type === 'ok' ? 'dd-toast-ok' : 'dd-toast-err'}`}>
      {msg.text}
    </div>
  )
}

function TabModes({ activeMode, setActiveMode, tenantId, deviceId }) {
  const [toast, setToast] = useState(null)
  const toastTimer = useRef(null)

  function showToast(type, text) {
    setToast({ type, text })
    clearTimeout(toastTimer.current)
    toastTimer.current = setTimeout(() => setToast(null), 3000)
  }

  async function handleSelect(mode) {
    if (mode.value === activeMode) return
    if (!window.confirm(`Превключване към ${mode.name}?`)) return
    try {
      await switchMode(tenantId, deviceId, mode.value)
      setActiveMode(mode.value)
      showToast('ok', 'Режимът е променен')
    } catch {
      showToast('err', 'Грешка при смяна на режима')
    }
  }

  const activeModeObj = ALL_MODES_FLAT.find((m) => m.value === activeMode)

  return (
    <div className="dd-tab-content">
      {activeModeObj && (
        <p className="dd-mode-current">
          Текущ режим: <strong>{activeModeObj.name}</strong>
        </p>
      )}

      {MODES.map((group) => (
        <div key={group.id} className="dd-mode-group">
          <p className="dd-mode-group-label">{group.label}</p>
          <div className="dd-mode-grid">
            {group.modes.map((mode) => {
              const isActive = mode.value === activeMode
              return (
                <div
                  key={mode.value}
                  className={`dd-mode-card${isActive ? ' dd-mode-card-active' : ''}`}
                  onClick={() => handleSelect(mode)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => e.key === 'Enter' && handleSelect(mode)}
                >
                  {isActive && <span className="dd-mode-check">✓</span>}
                  <p className="dd-mode-name">{mode.name}</p>
                  <p className="dd-mode-desc">{mode.desc}</p>
                  <p className="dd-mode-fixed">
                    {mode.fixed ? `Фиксирано: ${mode.fixed}` : 'Използва зададените настройки'}
                  </p>
                </div>
              )
            })}
          </div>
        </div>
      ))}

      <Toast msg={toast} />
    </div>
  )
}

// ── Tab: Диагностика ──────────────────────────────────────
function TabDiagnostics({ cycles, errors }) {
  const chartData = (cycles ?? []).map((c) => ({
    date:      formatChartDate(c.created_at),
    work_time: +(c.work_time / 60).toFixed(1),
    rest_time: +(c.rest_time / 60).toFixed(1),
  }))

  const activeErrors = (errors ?? []).filter((e) => e.active)

  const avgWork = chartData.length
    ? +(chartData.reduce((s, c) => s + c.work_time, 0) / chartData.length).toFixed(1)
    : 0
  const avgRest = chartData.length
    ? +(chartData.reduce((s, c) => s + c.rest_time, 0) / chartData.length).toFixed(1)
    : 0

  return (
    <div className="dd-tab-content">

      {/* ── Section 1: Compressor cycles ── */}
      <h3 className="dd-diag-section-title">Компресор</h3>
      {chartData.length === 0 ? (
        <p className="dd-empty">Няма данни за компресорни цикли</p>
      ) : (
        <>
          <div className="dd-chart-wrap">
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={chartData} margin={{ top: 8, right: 24, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#2a2d3a" />
                <XAxis dataKey="date" tick={{ fill: '#64748b', fontSize: 11 }} tickLine={false} />
                <YAxis
                  tick={{ fill: '#64748b', fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                  unit=" мин"
                />
                <Tooltip
                  contentStyle={{ background: '#1e2130', border: '1px solid #2a2d3a', borderRadius: 8, fontSize: '0.8125rem' }}
                  labelStyle={{ color: '#64748b', marginBottom: 4 }}
                  formatter={(value, name) => [`${value} мин`, name === 'work_time' ? 'Работа' : 'Почивка']}
                />
                <Legend
                  formatter={(value) => value === 'work_time' ? 'Работа' : 'Почивка'}
                  wrapperStyle={{ fontSize: '12px', color: '#94a3b8', paddingTop: '8px' }}
                />
                <Bar dataKey="work_time" fill="#3b82f6" radius={[3, 3, 0, 0]} />
                <Bar dataKey="rest_time" fill="#475569" radius={[3, 3, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="dd-diag-summary">
            <div className="dd-diag-summary-item">
              <span className="dd-diag-summary-label">Средна работа</span>
              <span className="dd-diag-summary-value dd-diag-work">{avgWork} мин</span>
            </div>
            <div className="dd-diag-summary-item">
              <span className="dd-diag-summary-label">Средна почивка</span>
              <span className="dd-diag-summary-value dd-diag-rest">{avgRest} мин</span>
            </div>
          </div>
        </>
      )}

      {/* ── Section 2: Active errors ── */}
      <h3 className="dd-diag-section-title dd-diag-section-title-errors">Грешки</h3>
      {activeErrors.length === 0 ? (
        <p className="dd-empty dd-no-errors">Няма активни грешки</p>
      ) : (
        <ul className="dd-error-list">
          {activeErrors.map((err, i) => (
            <li key={i} className="dd-error-item">
              <span className={`dd-sev-badge ${SEVERITY_CLASSES[err.severity] ?? 'sev-info'}`}>
                {SEVERITY_LABELS[err.severity] ?? 'Инфо'}
              </span>
              <span className="dd-error-message">{err.message}</span>
              <span className="dd-error-time">{relativeTime(err.timestamp)}</span>
            </li>
          ))}
        </ul>
      )}

    </div>
  )
}

// ── Tab: Логове ───────────────────────────────────────────
const LOGS_LINES_OPTIONS = [100, 200, 500]

function TabLogs({ tenantId, deviceId }) {
  const [lines, setLines] = useState(100)
  const [logLines, setLogLines] = useState([])
  const [loading, setLoading] = useState(false)
  const [fetchErr, setFetchErr] = useState('')
  const logsEndRef = useRef(null)

  const load = useCallback(async (n) => {
    setLoading(true)
    setFetchErr('')
    try {
      const res = await getDeviceLogs(tenantId, deviceId, n)
      setLogLines(res.data.lines ?? [])
    } catch {
      setFetchErr('Грешка при зареждане на логовете')
    } finally {
      setLoading(false)
    }
  }, [tenantId, deviceId])

  useEffect(() => { load(lines) }, [load, lines])

  useEffect(() => {
    if (!loading) logsEndRef.current?.scrollIntoView({ behavior: 'auto' })
  }, [logLines, loading])

  const handleLinesChange = (n) => {
    setLines(n)
  }

  return (
    <div className="dd-tab-content">
      <div className="dd-logs-toolbar">
        <div className="dd-logs-lines-group">
          {LOGS_LINES_OPTIONS.map((n) => (
            <button
              key={n}
              className={`dd-range-btn${lines === n ? ' dd-range-btn-active' : ''}`}
              onClick={() => handleLinesChange(n)}
              disabled={loading}
            >
              {n}
            </button>
          ))}
          <span className="dd-logs-lines-label">реда</span>
        </div>
        <button className="dd-logs-refresh-btn" onClick={() => load(lines)} disabled={loading}>
          Опресни
        </button>
      </div>

      {loading ? (
        <p className="dd-state-msg">Зареждане...</p>
      ) : fetchErr ? (
        <p className="dd-state-msg dd-error-msg">{fetchErr}</p>
      ) : logLines.length === 0 ? (
        <p className="dd-logs-empty">Няма налични логове</p>
      ) : (
        <div className="dd-logs-box">
          {logLines.map((line, i) => (
            <div key={i} className="dd-log-line">{line}</div>
          ))}
          <div ref={logsEndRef} />
        </div>
      )}
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────
export default function DeviceDetail() {
  const { token } = useAuth()
  const navigate = useNavigate()
  const { id: deviceId } = useParams()

  const claims = token ? decodeToken(token) : null
  const tenantId = claims?.tenant_id ?? null

  const [current, setCurrent] = useState(null)
  const [status, setStatus] = useState(null)
  const [history, setHistory] = useState([])
  const [settings, setSettings] = useState(null)
  const [rules, setRules] = useState([])
  const [activeMode, setActiveMode] = useState(null)
  const [cycles, setCycles] = useState([])
  const [errors, setErrors] = useState([])
  const [deviceTypes, setDeviceTypes] = useState([])
  const [deviceTypeId, setDeviceTypeId] = useState('')
  const [days, setDays] = useState(1)
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState('')
  const [activeTab, setActiveTab] = useState('history')

  // inline device-name edit
  const [nameEditing, setNameEditing] = useState(false)
  const [nameValue, setNameValue] = useState('')
  const [nameMsg, setNameMsg] = useState(null) // { type: 'ok'|'err', text }
  const nameMsgTimer = useRef(null)

  const fetchHistory = useCallback(async (d) => {
    if (!tenantId || !deviceId) return
    try {
      const res = await getHistory(tenantId, deviceId, d)
      setHistory(res.data?.readings ?? [])
    } catch (err) {
      console.error('DeviceDetail fetchHistory:', err)
    }
  }, [tenantId, deviceId])

  const fetchAll = useCallback(async () => {
    if (!tenantId || !deviceId) return
    try {
      const results = await Promise.allSettled([
        getCurrentReading(tenantId, deviceId),
        getDeviceStatus(tenantId, deviceId),
        getSettings(tenantId, deviceId),
        listAlertRules(tenantId, deviceId),
        getCompressorCycles(tenantId, deviceId, 7),
        getErrors(tenantId, deviceId),
        getDeviceTypes(),
        listDevices(tenantId),
      ])
      if (results[0].status === 'fulfilled') setCurrent(results[0].value.data)
      if (results[1].status === 'fulfilled') {
        const s = results[1].value.data
        setStatus(s)
        if (s?.active_mode != null) setActiveMode(MODE_STRING_TO_INT[s.active_mode] ?? 0)
      }
      if (results[2].status === 'fulfilled') setSettings(results[2].value.data)
      if (results[3].status === 'fulfilled') setRules(results[3].value.data ?? [])
      if (results[4].status === 'fulfilled') setCycles(results[4].value.data?.cycles ?? [])
      if (results[5].status === 'fulfilled') setErrors(results[5].value.data ?? [])
      if (results[6].status === 'fulfilled') setDeviceTypes(results[6].value.data ?? [])
      if (results[7].status === 'fulfilled') {
        const match = results[7].value.data?.find((d) => d.device_id === deviceId)
        if (match?.device_type_id) setDeviceTypeId(match.device_type_id)
      }
      setFetchError('')
    } catch (err) {
      console.error('DeviceDetail fetchAll:', err)
      setFetchError('Грешка при зареждане.')
    } finally {
      setLoading(false)
    }
  }, [tenantId, deviceId]) // intentionally excludes `days` — fetchHistory handles day changes

  const pollStatus = useCallback(async () => {
    if (!tenantId || !deviceId) return
    try {
      const [currentRes, statusRes] = await Promise.allSettled([
        getCurrentReading(tenantId, deviceId),
        getDeviceStatus(tenantId, deviceId),
      ])
      if (currentRes.status === 'fulfilled') setCurrent(currentRes.value.data)
      if (statusRes.status === 'fulfilled') {
        const s = statusRes.value.data
        setStatus(s)
        if (s?.active_mode != null) setActiveMode(MODE_STRING_TO_INT[s.active_mode] ?? 0)
      }
    } catch (err) {
      console.error('DeviceDetail pollStatus:', err)
    }
  }, [tenantId, deviceId])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  useEffect(() => {
    const id = setInterval(pollStatus, 30_000)
    return () => clearInterval(id)
  }, [pollStatus])

  useEffect(() => {
    fetchHistory(days)
  }, [fetchHistory, days])

  const isAdmin = claims?.role === 'admin'
  const alertActive = hasActiveError(status?.errors ?? [])
  const deviceName = status?.device_name || deviceId

  const handleNameEdit = () => {
    setNameValue(status?.device_name || '')
    setNameEditing(true)
  }

  const handleNameCancel = () => {
    setNameEditing(false)
    setNameValue('')
  }

  const handleNameConfirm = async () => {
    const trimmed = nameValue.trim()
    if (!trimmed) return
    try {
      await updateDeviceName(tenantId, deviceId, trimmed)
      setStatus((prev) => prev ? { ...prev, device_name: trimmed } : prev)
      setNameEditing(false)
      setNameMsg({ type: 'ok', text: 'Името е запазено' })
      clearTimeout(nameMsgTimer.current)
      nameMsgTimer.current = setTimeout(() => setNameMsg(null), 3000)
    } catch (err) {
      setNameMsg({ type: 'err', text: err.response?.data || 'Грешка при запазване' })
      clearTimeout(nameMsgTimer.current)
      nameMsgTimer.current = setTimeout(() => setNameMsg(null), 3000)
    }
  }
  const health = current?.health ?? null   // 0=Good, 1=Warning, 2=Error/Offline
  const isOffline = current?.temperature == null || health === 2
  const isStale   = !isOffline && health === 1

  if (loading) {
    return (
      <div className="dd-page">
        <p className="dd-state-msg">Зареждане...</p>
      </div>
    )
  }

  if (fetchError) {
    return (
      <div className="dd-page">
        <p className="dd-state-msg dd-error-msg">{fetchError}</p>
      </div>
    )
  }

  return (
    <div className="dd-page">
      <header className="dd-header">
        <div className="dd-header-left">
          <button className="dd-back-btn" onClick={() => navigate('/')}>← Назад</button>
          {nameEditing ? (
            <span className="dd-name-edit">
              <input
                className="dd-name-input"
                value={nameValue}
                onChange={(e) => setNameValue(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') handleNameConfirm(); if (e.key === 'Escape') handleNameCancel() }}
                autoFocus
                maxLength={64}
              />
              <button className="dd-name-btn dd-name-confirm" onClick={handleNameConfirm} title="Потвърди">✓</button>
              <button className="dd-name-btn dd-name-cancel" onClick={handleNameCancel} title="Откажи">✗</button>
            </span>
          ) : (
            <span className="dd-device-name">
              {deviceName}
              {isAdmin && (
                <button className="dd-name-edit-btn" onClick={handleNameEdit} title="Редактирай името">✏️</button>
              )}
            </span>
          )}
          {nameMsg && (
            <span className={`dd-name-msg dd-name-msg-${nameMsg.type}`}>{nameMsg.text}</span>
          )}
          {isOffline && <span className="dd-health-badge dd-health-offline">Офлайн</span>}
          {isStale   && <span className="dd-health-badge dd-health-stale">Стар сигнал</span>}
          <span className={`alert-badge ${alertActive ? 'alert-active' : 'alert-ok'}`}>
            {alertActive ? 'Алерт' : 'OK'}
          </span>
        </div>
      </header>

      <Tabs active={activeTab} onChange={setActiveTab} />

      {activeTab === 'history' && (
        <TabHistory current={current} status={status} history={history} days={days} setDays={setDays} />
      )}
      {activeTab === 'settings' && (
        <TabSettings
          settings={settings}
          tenantId={tenantId}
          deviceId={deviceId}
          deviceTypes={deviceTypes}
          deviceTypeId={deviceTypeId}
          setDeviceTypeId={setDeviceTypeId}
          isAdmin={isAdmin}
        />
      )}
      {activeTab === 'alerts' && (
        <TabAlerts rules={rules} setRules={setRules} tenantId={tenantId} deviceId={deviceId} />
      )}
      {activeTab === 'modes' && (
        <TabModes activeMode={activeMode} setActiveMode={setActiveMode} tenantId={tenantId} deviceId={deviceId} />
      )}
      {activeTab === 'diagnostics' && (
        <TabDiagnostics cycles={cycles} errors={errors} />
      )}
      {activeTab === 'logs' && (
        <TabLogs tenantId={tenantId} deviceId={deviceId} />
      )}
    </div>
  )
}
