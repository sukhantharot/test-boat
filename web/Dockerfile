# syntax=docker/dockerfile:1
FROM node:22-alpine AS build
WORKDIR /app

COPY package*.json ./
RUN npm ci || npm install

COPY . .
# PUBLIC_* ต้องมีตอน build เพราะถูก inline เข้าไปใน bundle ฝั่ง client
ARG PUBLIC_SITE_URL
ARG PUBLIC_API_URL
ENV PUBLIC_SITE_URL=$PUBLIC_SITE_URL
ENV PUBLIC_API_URL=$PUBLIC_API_URL
RUN npm run build

FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production

COPY package*.json ./
RUN npm ci --omit=dev || npm install --omit=dev
COPY --from=build /app/dist ./dist

# ต้องเป็น :: ไม่ใช่ 0.0.0.0 — Railway private network เป็น IPv6
# Node จะ bind dual-stack ให้เอง จึงเข้าถึงได้ทั้ง public และ private
ENV HOST=::
ENV PORT=4321
EXPOSE 4321
CMD ["node", "./dist/server/entry.mjs"]
