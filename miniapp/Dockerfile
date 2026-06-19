FROM node:20-alpine AS builder

WORKDIR /workspace/miniapp

COPY miniapp/package*.json ./
RUN npm ci

COPY miniapp/ ./
COPY shared/ ../shared/

ARG VITE_API_BASE_URL
ARG VITE_ALLOW_EMPTY_INITDATA
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
ENV VITE_ALLOW_EMPTY_INITDATA=$VITE_ALLOW_EMPTY_INITDATA

RUN npm run build

FROM nginx:1.27-alpine

ENV BACKEND_UPSTREAM=http://tglpbot:8080

COPY miniapp/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=builder /workspace/miniapp/dist /usr/share/nginx/html

EXPOSE 80
