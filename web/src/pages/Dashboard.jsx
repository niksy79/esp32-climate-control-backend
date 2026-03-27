import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useWebSocket } from '../hooks/useWebSocket'
import { listDevices, getCurrentReading, getDeviceStatus, getDeviceTypes, setLight, deleteDevice } from '../api/index'
import { formatTemperature, formatHumidity, decodeToken, relativeTime } from '../utils/index'
import './Dashboard.css'

// Device type ID → image import map.
// Add entries here as new device types / photos are introduced.
// Place image files in src/assets/devices/.
import meatDryerImg from '../assets/devices/meat-dryer.png'

const DEVICE_IMAGE_MAP = {
  climate_controller: meatDryerImg,
}

const MODE_LABELS = {
  normal:               'Нормален',
  heating:              'Нагряване',
  beer_cooling:         'Бира',
  room_temp:            'Стайна t°',
  product_meat_fish:    'Месо/Риба',
  product_dairy:        'Млечни',
  product_ready_food:   'Готова храна',
  product_vegetables:   'Зеленчуци',
}

const REFRESH_INTERVAL_MS = 60_000
const TEMP_THRESHOLD = 8.0

function RelayBadge({ label, on }) {
  return (
    <span className={`relay-badge ${on ? 'relay-on' : 'relay-off'}`}>
      {label}
    </span>
  )
}

function DeviceCard({ device, deviceTypes, isAdmin, onLightToggle, onDelete, onClick }) {
  const health    = device.health
  const isOffline = device.temperature === null || health === 2
  const isStale   = !isOffline && health === 1

  const compressorOn = device.deviceStates?.compressor  ?? false
  const extraFanOn   = device.deviceStates?.extra_fan   ?? false
  const lightOn      = device.deviceStates?.light       ?? false
  const heatingOn    = device.deviceStates?.heating     ?? false
  const hasErrors    = device.hasErrors
  const name         = device.deviceName || device.deviceId
  const tempHigh     = !isOffline && device.temperature > TEMP_THRESHOLD

  const deviceTypeObj = device.deviceTypeId
    ? deviceTypes?.find((t) => t.id === device.deviceTypeId)
    : null

  // Image: try by device type id, then by display name, then null (CSS placeholder)
  const deviceImage = DEVICE_IMAGE_MAP[device.deviceTypeId] ?? null

  let badgeClass, badgeLabel
  if (isOffline) {
    badgeClass = 'alert-active';  badgeLabel = 'Офлайн'
  } else if (device.alertFiring) {
    badgeClass = 'alert-active';  badgeLabel = 'Алерт'
  } else if (hasErrors) {
    badgeClass = 'alert-warning'; badgeLabel = 'Грешка'
  } else {
    badgeClass = 'alert-ok';      badgeLabel = 'OK'
  }

  const modeLabel = MODE_LABELS[device.activeMode] ?? '—'
  const fanSpeed  = device.fanSpeed

  const photoStyle = deviceImage ? { backgroundImage: `url(${deviceImage})` } : {}

  return (
    <div
      className="device-card"
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
    >
      {/* ── Photo half ── */}
      <div
        className={`card-photo-area${isOffline ? ' card-photo-offline' : ''}`}
        style={photoStyle}
      >
        <div className="card-photo-gradient" />

        {/* Badge — top right */}
        <span className={`card-badge-corner alert-badge ${badgeClass}`}>{badgeLabel}</span>

        {/* Device name — bottom left */}
        <div className="card-name-block">
          <span className="card-photo-name">{name}</span>
          {deviceTypeObj && (
            <span className="card-photo-type">{deviceTypeObj.display_name}</span>
          )}
        </div>
      </div>

      {/* ── Body half ── */}
      <div className="card-body">
        {/* 2×2 metric grid */}
        <div className="card-metrics">
          <div className="metric-cell">
            <span className="metric-label">Температура</span>
            <span className={
              `metric-value metric-temp` +
              (tempHigh && !isOffline ? ' temp-high' : '') +
              (isOffline ? ' metric-offline' : '')
            }>
              {isOffline ? '—' : formatTemperature(device.temperature)}
            </span>
          </div>

          <div className="metric-cell">
            <span className="metric-label">Влажност</span>
            <span className={
              `metric-value metric-hum` +
              (isStale ? ' reading-stale' : '') +
              (isOffline ? ' metric-offline' : '')
            }>
              {isOffline ? '—' : formatHumidity(device.humidity)}
            </span>
          </div>

          <div className="metric-cell">
            <span className="metric-label">Вентилатор</span>
            <span className="metric-value metric-fan">
              {!isOffline && fanSpeed != null ? `${fanSpeed}%` : '—'}
            </span>
          </div>

          <div className="metric-cell">
            <span className="metric-label">Режим</span>
            <span className="metric-value metric-mode">
              {isOffline ? '—' : modeLabel}
            </span>
          </div>
        </div>

        {/* Relay indicator badges */}
        <div className="card-relays">
          <RelayBadge label="Компресор" on={compressorOn} />
          <RelayBadge label="Вентилатор" on={extraFanOn} />
          <RelayBadge label="Нагревател" on={heatingOn} />
        </div>

        {/* Light toggle — full-width row, visually distinct from indicators */}
        <div className="card-light-row">
          {isAdmin ? (
            <button
              className={`card-light-toggle ${lightOn ? 'card-light-toggle-on' : 'card-light-toggle-off'}`}
              title={lightOn ? 'Изключи осветлението' : 'Включи осветлението'}
              onClick={(e) => { e.stopPropagation(); onLightToggle?.() }}
            >
              💡 Светлина&nbsp;<span className="light-state-label">{lightOn ? 'ON' : 'OFF'}</span>
            </button>
          ) : (
            <span className={`card-light-readonly ${lightOn ? 'card-light-toggle-on' : 'card-light-toggle-off'}`}>
              💡 Светлина&nbsp;<span className="light-state-label">{lightOn ? 'ON' : 'OFF'}</span>
            </span>
          )}
        </div>

        {/* Footer */}
        <div className="card-footer">
          <span>{relativeTime(device.timestamp)}</span>
          {isStale && <span className="stale-indicator">· Стар сигнал</span>}
          {isAdmin && (
            <button
              className="card-delete-btn"
              title="Изтрий устройство"
              onClick={(e) => {
                e.stopPropagation()
                if (window.confirm(`Изтрий устройство "${device.deviceName || device.deviceId}"?`)) {
                  onDelete?.()
                }
              }}
            >
              🗑
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

export default function Dashboard() {
  const { token } = useAuth()
  const navigate = useNavigate()

  const claims   = token ? decodeToken(token) : null
  const tenantId = claims?.tenant_id ?? null
  const isAdmin  = claims?.role === 'admin'

  const [devices,     setDevices]     = useState([])
  const [deviceTypes, setDeviceTypes] = useState([])
  const [loading,     setLoading]     = useState(true)
  const [fetchError,  setFetchError]  = useState('')

  const devicesRef = useRef(devices)
  devicesRef.current = devices

  const fetchAll = useCallback(async () => {
    if (!tenantId) return

    const doFetch = async () => {
      const [deviceIdsRes, deviceTypesRes] = await Promise.allSettled([
        listDevices(tenantId),
        getDeviceTypes(),
      ])
      if (deviceTypesRes.status === 'fulfilled') {
        setDeviceTypes(deviceTypesRes.value.data ?? [])
      }
      if (deviceIdsRes.status !== 'fulfilled') throw deviceIdsRes.reason
      const { data: deviceList } = deviceIdsRes.value  // [{device_id, device_type_id}]
      const enriched = await Promise.all(
        deviceList.map(async ({ device_id: deviceId, device_type_id: deviceTypeId }) => {
          const base = {
            deviceId,
            deviceTypeId:  deviceTypeId || null,
            deviceName:    null,
            temperature:   null,
            humidity:      null,
            timestamp:     null,
            health:        null,
            deviceStates:  null,
            systemState:   null,
            hasErrors:     false,
            alertFiring:   false,
            activeMode:    null,
            fanSpeed:      null,
          }
          try {
            const [currentRes, statusRes] = await Promise.allSettled([
              getCurrentReading(tenantId, deviceId),
              getDeviceStatus(tenantId, deviceId),
            ])
            const current = currentRes.status === 'fulfilled' ? currentRes.value.data : null
            const status  = statusRes.status  === 'fulfilled' ? statusRes.value.data  : null
            return {
              ...base,
              deviceName:   status?.device_name                ?? null,
              temperature:  current?.temperature               ?? null,
              humidity:     current?.humidity                  ?? null,
              timestamp:    current?.timestamp                 ?? null,
              health:       current?.health                    ?? null,
              deviceStates: status?.device_states              ?? null,
              systemState:  status?.system_status?.state       ?? null,
              hasErrors:    status?.has_errors                 ?? false,
              alertFiring:  status?.alert_firing               ?? false,
              activeMode:   status?.active_mode                ?? null,
              fanSpeed:     status?.fan_settings?.speed        ?? null,
            }
          } catch (err) {
            console.error(`fetchAll: device ${deviceId}:`, err)
            return base
          }
        })
      )
      setDevices(enriched)
      setFetchError('')
    }

    try {
      await doFetch()
    } catch (err) {
      if (err.response?.status === 401) {
        await new Promise((res) => setTimeout(res, 1000))
        try {
          await doFetch()
        } catch (retryErr) {
          console.error('fetchAll: retry failed:', retryErr)
          setFetchError(retryErr.response?.data?.error || 'Грешка при зареждане на устройствата')
        }
      } else {
        console.error('fetchAll: listDevices failed:', err)
        setFetchError(err.response?.data?.error || 'Грешка при зареждане на устройствата')
      }
    } finally {
      setLoading(false)
    }
  }, [tenantId])

  // Initial fetch + polling
  useEffect(() => {
    fetchAll()
    const id = setInterval(fetchAll, REFRESH_INTERVAL_MS)
    return () => clearInterval(id)
  }, [fetchAll])

  // WebSocket live sensor updates
  const { lastMessage, readyState } = useWebSocket(tenantId, token)
  useEffect(() => {
    if (!lastMessage || lastMessage.type !== 'sensor') return
    setDevices((prev) =>
      prev.map((d) =>
        d.deviceId === lastMessage.device_id
          ? {
              ...d,
              temperature: lastMessage.temperature,
              humidity:    lastMessage.humidity,
              timestamp:   lastMessage.timestamp,
            }
          : d
      )
    )
  }, [lastMessage])

  function handleLightToggle(deviceId, currentLightOn) {
    const newState = !currentLightOn
    setLight(tenantId, deviceId, { state: newState })
      .then(() => {
        setDevices((prev) =>
          prev.map((d) =>
            d.deviceId === deviceId
              ? { ...d, deviceStates: { ...d.deviceStates, light: newState } }
              : d
          )
        )
      })
      .catch((err) => console.error('light toggle:', err.response?.status, err.response?.data))
  }

  const isLive = readyState === WebSocket.OPEN

  return (
    <div className="dashboard">
      <header className="dashboard-header">
        <div className="header-spacer" />
        <div className="header-center">
          <h1 className="dashboard-title">Дашборд</h1>
          <span className="live-indicator">
            <span className={`live-dot ${isLive ? 'live-dot-on' : 'live-dot-off'}`} />
            {isLive ? 'Свързан' : 'Без връзка'}
          </span>
        </div>
        <div className="header-actions">
          <button className="profile-btn" onClick={() => navigate('/profile')}>Профил</button>
        </div>
      </header>

      <main className="dashboard-main">
        {loading && <p className="state-msg">Зареждане на устройствата...</p>}

        {!loading && fetchError && (
          <p className="state-msg error-msg">{fetchError}</p>
        )}

        {!loading && !fetchError && devices.length === 0 && (
          <p className="state-msg">Няма намерени устройства.</p>
        )}

        {!loading && !fetchError && devices.length > 0 && (
          <div className="device-grid">
            {devices.map((d) => (
              <DeviceCard
                key={d.deviceId}
                device={d}
                deviceTypes={deviceTypes}
                isAdmin={isAdmin}
                onLightToggle={() => handleLightToggle(d.deviceId, d.deviceStates?.light ?? false)}
                onDelete={async () => {
                  await deleteDevice(tenantId, d.deviceId)
                  setDevices((prev) => prev.filter((x) => x.deviceId !== d.deviceId))
                }}
                onClick={() => navigate(`/device/${d.deviceId}`)}
              />
            ))}
          </div>
        )}
      </main>
    </div>
  )
}
