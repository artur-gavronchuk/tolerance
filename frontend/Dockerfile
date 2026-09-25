# Agent Arena web (Next.js, standalone output).
FROM node:24-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm config set store-dir /root/.local/share/pnpm/store && pnpm install --frozen-lockfile
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
# next.config.mjs reads API_URL at build time: the /api rewrite target is
# written into the routes manifest, so a runtime env var would not change it.
ARG API_URL=http://api:8080
ENV API_URL=$API_URL
# The contact address on /terms, inlined into the build the same way.
ARG NEXT_PUBLIC_CONTACT_EMAIL=
ENV NEXT_PUBLIC_CONTACT_EMAIL=$NEXT_PUBLIC_CONTACT_EMAIL
# Cloudflare Turnstile site key for the signup form, inlined the same way.
# Empty renders no widget and sends nothing extra (dev unchanged).
ARG NEXT_PUBLIC_TURNSTILE_SITE_KEY=
ENV NEXT_PUBLIC_TURNSTILE_SITE_KEY=$NEXT_PUBLIC_TURNSTILE_SITE_KEY
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
