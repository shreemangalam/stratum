FROM node:24-alpine AS deps

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

FROM node:24-alpine AS builder

WORKDIR /app
ARG NEXT_PUBLIC_API_URL=http://localhost:8080
ARG NEXT_PUBLIC_GIT_ENABLED=false
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_GIT_ENABLED=$NEXT_PUBLIC_GIT_ENABLED
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:24-alpine

WORKDIR /app

COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

USER node
EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME=0.0.0.0
ENV NODE_ENV=production

CMD ["node", "server.js"]
