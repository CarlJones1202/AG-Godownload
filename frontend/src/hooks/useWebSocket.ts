import { useState, useEffect, useRef } from 'react';
import type { DownloadStatus } from '@/types';

const RECONNECT_DELAY = 3000;

export function useWebSocket() {
  const [status, setStatus] = useState<DownloadStatus | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    let unmounted = false;

    const connect = () => {
      if (unmounted || wsRef.current) return;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws`;
      const socket = new WebSocket(wsUrl);
      wsRef.current = socket;

      socket.onopen = () => {
        // This socket may have been superseded by cleanup (StrictMode remount).
        if (unmounted || wsRef.current !== socket) {
          socket.close();
          return;
        }
        setConnected(true);
      };

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
        if (wsRef.current !== socket) return;
        wsRef.current = null;
        if (unmounted) return;
        setConnected(false);
        reconnectTimer.current = setTimeout(connect, RECONNECT_DELAY);
      };

      socket.onerror = () => socket.close();
    };

    connect();

    return () => {
      unmounted = true;
      clearTimeout(reconnectTimer.current);
      reconnectTimer.current = undefined;
      const socket = wsRef.current;
      wsRef.current = null;
      if (!socket) return;
      if (socket.readyState === WebSocket.CONNECTING) {
        // Closing a CONNECTING socket makes the browser log a network-level
        // "WebSocket is closed before the connection is established" error.
        // Defer the close until the connection completes instead.
        socket.onopen = () => socket.close();
      } else {
        socket.close();
      }
    };
  }, []);

  return { status, connected };
}