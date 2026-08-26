# syntax=docker/dockerfile:1
#
# The admin panel image. The build context is this module directory, which is
# self-contained: one package, no workspace to reach out of.
#
#   docker build -t admin-web:dev admin/web

FROM node:26-alpine AS deps
WORKDIR /src
# Manifests only, so a source-only change reuses the install layer.
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

FROM node:26-alpine AS build
WORKDIR /src
COPY --from=deps /src/node_modules ./node_modules
COPY . .
# Telemetry is off because a build should not phone home from CI.
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# `output: "standalone"` (next.config.ts) emits a server bundle carrying only the
# dependencies it actually traced, so the runtime image installs nothing. The
# static assets and public/ are not part of that bundle and are copied beside it.
FROM node:26-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# 65532:65532 matches the chart's securityContext, and node:alpine has no user at
# that id — so it is created here rather than assumed.
RUN addgroup -g 65532 -S nonroot && adduser -u 65532 -S -G nonroot nonroot

COPY --from=build --chown=nonroot:nonroot /src/.next/standalone ./
COPY --from=build --chown=nonroot:nonroot /src/.next/static ./.next/static

EXPOSE 3000
USER nonroot:nonroot
CMD ["node", "server.js"]
