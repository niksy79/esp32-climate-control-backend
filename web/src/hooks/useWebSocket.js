import { useEffect, useRef, useState } from 'react'

export function useWebSocket(tenantId, token) {
  const [lastMessage, setLastMessage] = useState(null)
  const [readyState, setReadyState] = useState(WebSocket.CONNECTING)
  const wsRef = useRef(null)

  useEffect(() => {
    if (!tenantId || !token) return

    const apiUrl = import.meta.env.VITE_API_URL || ''
    const wsBase = apiUrl
      ? apiUrl.replace('https://', 'wss://').replace('http://', 'ws://')
      : `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}`
    const url = `${wsBase}/ws/${tenantId}?token=${token}`
    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen  = () => setReadyState(WebSocket.OPEN)
    ws.onclose = () => { if (wsRef.current === ws) setReadyState(WebSocket.CLOSED) }
    ws.onerror = () => { if (wsRef.current === ws) setReadyState(WebSocket.CLOSED) }
    ws.onmessage = (event) => {
      try {
        setLastMessage(JSON.parse(event.data))
      } catch {
        setLastMessage(event.data)
      }
    }

    return () => ws.close()
  }, [tenantId, token])

  return { lastMessage, readyState }
}
