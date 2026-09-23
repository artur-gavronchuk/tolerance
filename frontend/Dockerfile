# Agent Arena web (Next.js, standalone output).
FROM node:24-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm config set store-dir /root/.local/share/pnpm/store && pnpm install --frozen-lockfile
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM node:24-alpine
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
RUN adduser -D -u 10001 arena
COPY --from=build --chown=arena:arena /app/.next/standalone ./
COPY --from=build --chown=arena:arena /app/.next/static ./.next/static
COPY --from=build --chown=arena:arena /app/public ./public
USER arena
EXPOSE 3000
CMD ["node", "server.js"]
