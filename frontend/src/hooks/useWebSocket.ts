import { useState, useEffect, useRef, useCallback } from 'react';
import type { DownloadStatus } from '@/types';

export function useWebSocket() {
  const [status, setStatus] = useState<DownloadStatus | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const connect = useCallback(() => {
    if (wsRef.current) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;
    const socket = new WebSocket(wsUrl);

    socket.onopen = () => setConnected(true);

    socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as DownloadStatus;
        if (data.crawler || data.verification || data.videos) {
          setStatus(data);
        }
      } catch {
        // Ignore malformed messages
      }
    };

    socket.onclose = () => {
      setConnected(false);
      wsRef.current = null;
      reconnectTimer.current = setTimeout(connect, 3000);
    };

    socket.onerror = () => socket.close();
    wsRef.current = socket;
  }, []);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [connect]);

  return { status, connected };
}
