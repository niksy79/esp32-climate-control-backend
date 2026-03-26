import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useWebSocket } from '../hooks/useWebSocket'
import { listDevices, getCurrentReading, getDeviceStatus, getDeviceTypes, setLight } from '../api/index'
import { formatTemperature, formatHumidity, decodeToken, relativeTime } from '../utils/index'
import './Dashboard.css'

const SYSTEM_STATE_LABELS = ['Нормален', 'Предупреждение', 'Грешка', 'Безопасен режим', 'Резервен']
const REFRESH_INTERVAL_MS = 60_000
const TEMP_THRESHOLD = 8.0

function hasActiveError(errors) {
  return Array.isArray(errors) && errors.some((e) => e.active)
}

function RelayBadge({ label, on }) {
  return (
    <span className={`relay-badge ${on ? 'relay-on' : 'relay-off'}`}>
      {label}&nbsp;{on ? 'ON' : 'OFF'}
    </span>
  )
}

function DeviceCard({ device, deviceTypes, isAdmin, onLightToggle, onClick }) {
  const health = device.health   // 0=Good, 1=Warning, 2=Error/Offline
  const isOffline = device.temperature === null || health === 2
  const isStale   = !isOffline && health === 1
  const compressorOn = device.deviceStates?.compressor ?? false
  const fanOn = device.deviceStates?.extra_fan ?? false
  const lightOn = device.deviceStates?.light ?? false
  const heatingOn = device.deviceStates?.heating ?? false
  const alertActive = hasActiveError(device.errors)
  const stateLabel = SYSTEM_STATE_LABELS[device.systemState] ?? 'Неизвестно'
  const name = device.deviceName || device.deviceId
  const tempHigh = !isOffline && device.temperature > TEMP_THRESHOLD
  const deviceType = device.deviceTypeId
    ? deviceTypes?.find((t) => t.id === device.deviceTypeId)
    : null

  return (
    <div
      className="device-card"
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
    >
      <div className="card-header">
        <div className="card-title-group">
          <span className="card-title">{name}</span>
          {deviceType && <span className="card-device-type">{deviceType.display_name}</span>}
        </div>
        <span className={`alert-badge ${isOffline || alertActive ? 'alert-active' : 'alert-ok'}`}>
          {isOffline ? 'Офлайн' : alertActive ? 'Алерт' : 'OK'}
        </span>
      </div>

      <div className="card-readings">
        {isOffline ? (
          <div className="offline-row">
            <span className="offline-badge">Офлайн</span>
            <span className="reading-temp offline-dash">—</span>
          </div>
        ) : (
          <span className={`reading-temp${tempHigh ? ' temp-high' : ''}`}>
            {formatTemperature(device.temperature)}
          </span>
        )}
        <span className={`reading-hum${isOffline ? '' : isStale ? ' reading-stale' : ''}`}>
          {isOffline ? '— %' : formatHumidity(device.humidity)}
        </span>
      </div>

      <div className="relay-badges">
        <RelayBadge label="Компресор" on={compressorOn} />
        <RelayBadge label="Вентилатор" on={fanOn} />
        <RelayBadge label="Нагревател" on={heatingOn} />
        <button
          className={`card-light-btn ${lightOn ? 'card-light-on' : 'card-light-off'}`}
          disabled={!isAdmin}
          title={!isAdmin ? undefined : lightOn ? 'Изключи осветлението' : 'Включи осветлението'}
          onClick={(e) => { e.stopPropagation(); onLightToggle && onLightToggle() }}
        >
          {lightOn ? 'Светлина ON' : 'Светлина OFF'}
        </button>
      </div>

      <div className="card-state">{stateLabel}</div>

      <div className="card-footer">
        {relativeTime(device.timestamp)}
        {isStale && <span className="stale-indicator">· Стар сигнал</span>}
      </div>
    </div>
  )
}

export default function Dashboard() {
  const { token, logout } = useAuth()
  const navigate = useNavigate()

  const claims = token ? decodeToken(token) : null
  const tenantId = claims?.tenant_id ?? null
  const isAdmin = claims?.role === 'admin'

  const [devices, setDevices] = useState([])
  const [deviceTypes, setDeviceTypes] = useState([])
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState('')

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
            deviceTypeId: deviceTypeId || null,
            deviceName: null,
            temperature: null,
            humidity: null,
            timestamp: null,
            health: null,
            deviceStates: null,
            systemState: null,
            errors: [],
          }
          try {
            const [currentRes, statusRes] = await Promise.allSettled([
              getCurrentReading(tenantId, deviceId),
              getDeviceStatus(tenantId, deviceId),
            ])
            const current = currentRes.status === 'fulfilled' ? currentRes.value.data : null
            const status = statusRes.status === 'fulfilled' ? statusRes.value.data : null
            return {
              ...base,
              deviceName: status?.device_name ?? null,
              temperature: current?.temperature ?? null,
              humidity: current?.humidity ?? null,
              timestamp: current?.timestamp ?? null,
              health: current?.health ?? null,
              deviceStates: status?.device_states ?? null,
              systemState: status?.system_status?.state ?? null,
              errors: status?.errors ?? [],
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
        // Give the axios interceptor time to refresh the token, then retry once.
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

  // Initial fetch + 30s polling
  useEffect(() => {
    fetchAll()
    const id = setInterval(fetchAll, REFRESH_INTERVAL_MS)
    return () => clearInterval(id)
  }, [fetchAll])

  // WebSocket live updates
  const { lastMessage, readyState } = useWebSocket(tenantId, token)
  useEffect(() => {
    if (!lastMessage || lastMessage.type !== 'sensor') return
    setDevices((prev) =>
      prev.map((d) =>
        d.deviceId === lastMessage.device_id
          ? {
              ...d,
              temperature: lastMessage.temperature,
              humidity: lastMessage.humidity,
              timestamp: lastMessage.timestamp,
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

  function handleLogout() {
    logout()
    navigate('/login')
  }

  const isLive = readyState === WebSocket.OPEN

  return (
    <div className="dashboard">
      <header className="dashboard-header">
        <div className="header-left">
          <h1 className="dashboard-title">Climate Control</h1>
          <span className="live-indicator">
            <span className={`live-dot ${isLive ? 'live-dot-on' : 'live-dot-off'}`} />
            Живо
          </span>
        </div>
        <div className="header-actions">
          <button className="profile-btn" onClick={() => navigate('/profile')}>Профил</button>
          <button className="logout-btn" onClick={handleLogout}>Изход</button>
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
                onClick={() => navigate(`/device/${d.deviceId}`)}
              />
            ))}
          </div>
        )}
      </main>
    </div>
  )
}
