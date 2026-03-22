// k6/ws_test.js
// Нагрузочный тест WebSocket соединений
// Запуск: k6 run k6/ws_test.js

import http from "k6/http";
import ws from "k6/ws";
import { check, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const WS_URL   = __ENV.WS_URL   || "ws://localhost:8080/ws";

const msgReceived = new Counter("ws_messages_received");
const wsLatency   = new Trend("ws_message_latency");

export const options = {
  scenarios: {
    ws_connections: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 100  },
        { duration: "2m",  target: 100  },
        { duration: "30s", target: 500  },
        { duration: "2m",  target: 500  },
        { duration: "1m",  target: 1000 },
        { duration: "2m",  target: 1000 },
        { duration: "30s", target: 0    },
      ],
    },
  },
  thresholds: {
    // 99% WS соединений держатся без ошибок
    ws_session_duration: ["p(95)<30000"],
    ws_messages_received: ["count>0"],
  },
};

// Логин и получение токена
function getToken() {
  const username = `wstest_${__VU}_${Date.now()}`;
  http.post(
    `${BASE_URL}/api/v1/auth/register`,
    JSON.stringify({ username, password: "WsTest123!" }),
    { headers: { "Content-Type": "application/json" } }
  );
  const res = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ username, password: "WsTest123!" }),
    { headers: { "Content-Type": "application/json" } }
  );
  return res.json("token");
}

export default function () {
  const token = getToken();
  if (!token) return;

  const res = ws.connect(
    WS_URL,
    { headers: { Authorization: `Bearer ${token}` } },
    (socket) => {
      let pingTime = 0;

      socket.on("open", () => {
        // Ping каждые 5 секунд
        socket.setInterval(() => {
          pingTime = Date.now();
          socket.send(JSON.stringify({ type: "ping" }));
        }, 5000);
      });

      socket.on("message", (data) => {
        msgReceived.add(1);
        try {
          const msg = JSON.parse(data);
          if (msg.type === "pong" && pingTime > 0) {
            wsLatency.add(Date.now() - pingTime);
            pingTime = 0;
          }
        } catch (_) {}
      });

      socket.on("error", (e) => {
        console.log(`WS error VU ${__VU}: ${e}`);
      });

      // Держим соединение 20 секунд
      socket.setTimeout(() => socket.close(), 20000);
    }
  );

  check(res, { "ws: connected (101)": (r) => r && r.status === 101 });
  sleep(2);
}