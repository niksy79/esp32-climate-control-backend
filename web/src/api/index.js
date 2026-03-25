import client from './client'

// Auth
export const register = (data) => client.post('/api/auth/register', data)
export const login = (data) => client.post('/api/auth/login', data)
export const refresh = (data) => client.post('/api/auth/refresh', data)

// Devices
export const listDevices = (tenantId) =>
  client.get(`/api/tenants/${tenantId}/devices`)

export const getCurrentReading = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/current`)

export const getDeviceStatus = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/status`)

export const getHistory = (tenantId, deviceId, days = 1) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/history`, { params: { days } })

export const getCompressorCycles = (tenantId, deviceId, days = 7) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/compressor-cycles`, { params: { days } })

export const getErrors = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/errors`)

export const getSettings = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/settings`)

export const saveSettings = (tenantId, deviceId, data) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/settings`, data)

export const switchMode = (tenantId, deviceId, mode) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/mode`, { mode })

// Alert rules
export const listAlertRules = (tenantId, deviceId) =>
  client.get(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules`)

export const createAlertRule = (tenantId, deviceId, data) =>
  client.post(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules`, data)

export const updateAlertRule = (tenantId, deviceId, ruleId, data) =>
  client.put(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules/${ruleId}`, data)

export const deleteAlertRule = (tenantId, deviceId, ruleId) =>
  client.delete(`/api/tenants/${tenantId}/devices/${deviceId}/alert-rules/${ruleId}`)
