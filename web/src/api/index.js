import client from './client'

// Auth
export const register = (data) => client.post('/api/auth/register', data)
export const login = ({ email, password }) => client.post('/api/auth/login', { email, password })
export const refresh = (data) => client.post('/api/auth/refresh', data)

export const registerUser = (data) => client.post('/api/auth/register', data)
export const forgotPassword = (email) =>
  client.post('/api/auth/forgot-password', { email })
export const resetPassword = (token, password) =>
  client.post('/api/auth/reset-password', { token, password })
export const changePassword = (old_password, new_password) =>
  client.post('/api/auth/change-password', { old_password, new_password })

// Devices
export const listDevices = (tenantId) =>
  client.get(`/api/tenants/${tenantId}/devices`)

export const getCurrentReading = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/current`)

export const getDeviceStatus = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/status`)

export const getHistory = (tenantId, deviceId, days = 1) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/history`, { params: { days } })

export const getMetricHistory = (tenantId, deviceId, metric, days = 1) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/history`, {
    params: { metric, days },
  })

export const getCompressorCycles = (tenantId, deviceId, days = 7) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/compressor-cycles`, { params: { days } })

export const getErrors = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/errors`)

// Device types
export const getDeviceTypes = () =>
  client.get('/api/device-types')

export const setDeviceType = (tenantId, deviceId, deviceTypeId) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/type`, { device_type_id: deviceTypeId })

export const updateDeviceName = (tenantId, deviceId, name) =>
  client.patch(`/api/tenants/${tenantId}/devices/${deviceId}/name`, { device_name: name })

export const deleteDevice = (tenantId, deviceId) =>
  client.delete(`/api/tenants/${tenantId}/devices/${deviceId}`)

export const getDeviceLogs = (tenantId, deviceId, lines = 100) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/logs`, { params: { lines } })

export const getSettings = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/settings`)

export const saveSettings = (tenantId, deviceId, data) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/settings`, data)

export const switchMode = (tenantId, deviceId, mode) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/mode`, { mode })

export const setLight = (tenantId, deviceId, payload) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/light`, payload)

// User management
export const listUsers = (tenantId) =>
  client.get(`/api/tenants/${tenantId}/users`)

export const createUser = (tenantId, data) =>
  client.post(`/api/tenants/${tenantId}/users`, data)

export const deleteUser = (tenantId, userId) =>
  client.delete(`/api/tenants/${tenantId}/users/${userId}`)

// Alert rules
export const listAlertRules = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules`)

export const createAlertRule = (tenantId, deviceId, data) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules`, data)

export const updateAlertRule = (tenantId, deviceId, ruleId, data) =>
  client.put(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules/${ruleId}`, data)

export const deleteAlertRule = (tenantId, deviceId, ruleId) =>
  client.delete(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules/${ruleId}`)
