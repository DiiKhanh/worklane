# Build with the repository root as context.
FROM node:22-bookworm-slim
WORKDIR /app
RUN corepack enable && chown node:node /app
USER node
COPY --chown=node:node dashboard/package.json dashboard/pnpm-lock.yaml dashboard/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY --chown=node:node dashboard/ .
ARG NEXT_PUBLIC_API_BASE
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE \
    NEXT_PUBLIC_DATA_SOURCE=live \
    NEXT_TELEMETRY_DISABLED=1
RUN pnpm build
EXPOSE 3000
CMD ["pnpm", "start", "--hostname", "0.0.0.0"]
