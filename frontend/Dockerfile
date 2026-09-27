# Production image: builds the SPA and serves it with nginx, proxying /api and
# /healthz to the backend. The Vite dev-server image lives in Dockerfile.dev.

FROM node:22-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

RUN npm run build

FROM nginx:1.28-alpine

# Non-root runtime (uid 101): /etc/nginx/conf.d is not writable by the nginx
# user, so the entrypoint renders the template into /tmp/conf.d instead. The
# stock conf.d file is unused (nginx.conf includes /tmp/conf.d/*.conf).
RUN mkdir -p /tmp/conf.d && chown 101:101 /tmp/conf.d \
    && rm -f /etc/nginx/conf.d/default.conf

COPY nginx.conf /etc/nginx/nginx.conf
COPY default.conf.template /etc/nginx/templates/default.conf.template

ENV NGINX_ENVSUBST_OUTPUT_DIR=/tmp/conf.d \
    NGINX_ENTRYPOINT_LOCAL_RESOLVERS=true \
    BACKEND_ORIGIN=http://backend:8080

COPY --from=build --chown=101:101 /app/dist /usr/share/nginx/html

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/ || exit 1

USER 101
