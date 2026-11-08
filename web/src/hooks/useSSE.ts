import { useEffect, useCallback } from 'react';
import { useAuthStore } from '../store/auth';

export function useSSE(
  caseId: string | undefined,
  onEvent: (event: { type: string; case_id: string; version: number }) => void
) {
  const token = useAuthStore((s) => s.token);

  useEffect(() => {
    if (!caseId || !token) return;

    const eventSource = new EventSource(
      `${import.meta.env.VITE_API_BASE || 'http://localhost:8080'}/api/v1/cases/${caseId}/events`,
      {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      }
    );

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        onEvent(data);
      } catch (err) {
        console.error('Failed to parse SSE event:', err);
      }
    };

    eventSource.onerror = (err) => {
      console.error('SSE error:', err);
      eventSource.close();
    };

    return () => {
      eventSource.close();
    };
  }, [caseId, token, onEvent]);
}
