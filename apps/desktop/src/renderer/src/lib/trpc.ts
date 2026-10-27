import { QueryClient } from '@tanstack/react-query';
import { createTRPCClient, httpBatchLink } from '@trpc/client';
import { createTRPCContext } from '@trpc/tanstack-react-query';
import type { AppRouter } from '../../../main/router';

/** fetch() for the tRPC client that goes through Electron IPC instead of the network. */
async function ipcFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const request = new Request(input, init);
  const body = request.method === 'GET' || request.method === 'HEAD' ? null : await request.text();
  const res = await window.studio.trpc({
    url: request.url,
    method: request.method,
    headers: [...request.headers.entries()],
    body,
  });
  return new Response(res.body, { status: res.status, headers: res.headers });
}

export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 5_000, refetchOnWindowFocus: false, retry: false } },
});

export const trpcClient = createTRPCClient<AppRouter>({
  links: [httpBatchLink({ url: 'http://studio.local/trpc', fetch: ipcFetch })],
});

export const { TRPCProvider, useTRPC } = createTRPCContext<AppRouter>();
