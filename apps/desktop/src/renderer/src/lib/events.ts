import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useTRPC } from './trpc';

/** Refetches what changed when main reports a run, map or task update. */
export function useStudioEvents(): void {
  const trpc = useTRPC();
  const queryClient = useQueryClient();
  useEffect(
    () =>
      window.studio.onEvent((event) => {
        void queryClient.invalidateQueries({
          queryKey: trpc.map.get.queryKey({ projectId: event.projectId }),
        });
        void queryClient.invalidateQueries({
          queryKey: trpc.requests.list.queryKey({ projectId: event.projectId }),
        });
        if (event.type === 'run') {
          void queryClient.invalidateQueries({ queryKey: trpc.runs.events.pathKey() });
        }
      }),
    [queryClient, trpc],
  );
}
