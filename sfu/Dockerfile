FROM node:20-alpine

# mediasoup требует нативную компиляцию
RUN apk add --no-cache python3 make g++ linux-headers

# Non-root user — безопасность
RUN addgroup -S sfu && adduser -S sfu -G sfu

WORKDIR /app

COPY package.json ./
RUN npm install --omit=dev

COPY index.js room.js ./

# Healthcheck — проверяем что порт слушается
HEALTHCHECK --interval=15s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:4000/health || exit 1

RUN chown -R sfu:sfu /app
USER sfu

EXPOSE 4000

CMD ["node", "index.js"]
