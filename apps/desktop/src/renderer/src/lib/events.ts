import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useTRPC } from './trpc';

/** Refetches what changed when main reports a run, map, task or workspace update. */
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
        // Task diffs are read from git; they change when an implement run ends or a worktree goes.
        if (event.type === 'request') {
          void queryClient.invalidateQueries({ queryKey: trpc.tasks.diff.pathKey() });
        }
        if (event.type === 'workspace') {
          void queryClient.invalidateQueries({
            queryKey: trpc.workspaces.list.queryKey({ projectId: event.projectId }),
          });
          void queryClient.invalidateQueries({
            queryKey: trpc.workspaces.setupLog.queryKey({ workspaceId: event.workspaceId }),
          });
        }
        if (event.type === 'run') {
          void queryClient.invalidateQueries({ queryKey: trpc.runs.events.pathKey() });
        }
      }),
    [queryClient, trpc],
  );
}
