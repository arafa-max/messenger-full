// k6/load_test.js
// Запуск: k6 run k6/load_test.js
// С параметрами: k6 run --vus 50 --duration 30s k6/load_test.js

import http from "k6/http";
import ws from "k6/ws";
import { check, sleep, group } from "k6";
import { Rate, Trend, Counter } from "k6/metrics";

// ── Кастомные метрики ─────────────────────────────────────────
const errorRate      = new Rate("errors");
const wsConnectTime  = new Trend("ws_connect_time");
const msgSendTime    = new Trend("msg_send_time");
const authTime       = new Trend("auth_time");
const messagesTotal  = new Counter("messages_sent");

// ── Конфиг ────────────────────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const WS_URL   = __ENV.WS_URL   || "ws://localhost:8080/ws";

// ── Сценарии нагрузки ─────────────────────────────────────────
export const options = {
  scenarios: {
    // Сценарий 1: Разогрев — плавный рост
    warmup: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 10 },
        { duration: "30s", target: 10 },
        { duration: "10s", target: 0 },
      ],
      gracefulRampDown: "10s",
      tags: { scenario: "warmup" },
    },

    // Сценарий 2: Основная нагрузка
    load: {
      executor: "ramping-vus",
      startVUs: 0,
      startTime: "1m20s",  // после warmup
      stages: [
        { duration: "1m",  target: 50  },
        { duration: "3m",  target: 50  },
        { duration: "1m",  target: 100 },
        { duration: "3m",  target: 100 },
        { duration: "2m",  target: 0   },
      ],
      gracefulRampDown: "30s",
      tags: { scenario: "load" },
    },

    // Сценарий 3: Стресс-тест
    stress: {
      executor: "ramping-vus",
      startVUs: 0,
      startTime: "11m",
      stages: [
        { duration: "2m",  target: 200 },
        { duration: "5m",  target: 200 },
        { duration: "2m",  target: 500 },
        { duration: "3m",  target: 500 },
        { duration: "2m",  target: 0   },
      ],
      gracefulRampDown: "30s",
      tags: { scenario: "stress" },
    },
  },

  // Пороги — тест провалится если нарушены
  thresholds: {
    // 95% запросов быстрее 500мс
    http_req_duration: ["p(95)<500", "p(99)<1000"],
    // Ошибок меньше 1%
    errors: ["rate<0.01"],
    // WebSocket подключение быстрее 200мс
    ws_connect_time: ["p(95)<200"],
    // 99% HTTP запросов успешны
    http_req_failed: ["rate<0.01"],
  },
};

// ── Хелперы ───────────────────────────────────────────────────
function headers(token) {
  const h = { "Content-Type": "application/json" };
  if (token) h["Authorization"] = `Bearer ${token}`;
  return h;
}

function randomString(len) {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let s = "";
  for (let i = 0; i < len; i++)
    s += chars[Math.floor(Math.random() * chars.length)];
  return s;
}

// ── Регистрация и логин ───────────────────────────────────────
function authFlow() {
  const username = `user_${randomString(8)}`;
  const password = "TestPassword123!";

  const start = Date.now();

  // Регистрация
  const reg = http.post(
    `${BASE_URL}/api/v1/auth/register`,
    JSON.stringify({ username, password }),
    { headers: headers() }
  );

  const regOk = check(reg, {
    "register: status 201": (r) => r.status === 201,
  });
  errorRate.add(!regOk);

  if (!regOk) return null;

  // Логин
  const login = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ username, password }),
    { headers: headers() }
  );

  const loginOk = check(login, {
    "login: status 200":    (r) => r.status === 200,
    "login: has token":     (r) => r.json("token") !== undefined,
  });
  errorRate.add(!loginOk);

  authTime.add(Date.now() - start);

  if (!loginOk) return null;
  return login.json("token");
}

// ── Основной сценарий ─────────────────────────────────────────
export default function () {
  const token = authFlow();
  if (!token) return;

  group("REST API", () => {
    // Профиль
    group("profile", () => {
      const me = http.get(
        `${BASE_URL}/api/v1/auth/me`,
        { headers: headers(token) }
      );
      check(me, { "me: status 200": (r) => r.status === 200 });
      errorRate.add(me.status !== 200);
      sleep(0.5);
    });

    // Список чатов
    group("chats", () => {
      const chats = http.get(
        `${BASE_URL}/api/v1/chats`,
        { headers: headers(token) }
      );
      check(chats, { "chats: status 200": (r) => r.status === 200 });
      errorRate.add(chats.status !== 200);
      sleep(0.3);
    });

    // Health check
    group("health", () => {
      const health = http.get(`${BASE_URL}/health`);
      check(health, {
        "health: status 200": (r) => r.status === 200,
        "health: db ok":      (r) => r.json("db") === true,
      });
    });
  });

  // WebSocket сценарий (каждый 3й пользователь)
  if (__VU % 3 === 0) {
    group("WebSocket", () => {
      const start = Date.now();

      const res = ws.connect(
        `${WS_URL}`,
        { headers: { Authorization: `Bearer ${token}` } },
        (socket) => {
          wsConnectTime.add(Date.now() - start);

          socket.on("open", () => {
            // Отправляем typing индикатор
            socket.send(JSON.stringify({
              type: "typing",
              chat_id: 1,
            }));
          });

          socket.on("message", (data) => {
            const msg = JSON.parse(data);
            check(msg, { "ws: valid message": (m) => m.type !== undefined });
          });

          socket.on("error", (e) => {
            errorRate.add(1);
          });

          // Держим соединение 5 секунд
          socket.setTimeout(() => socket.close(), 5000);
        }
      );

      check(res, { "ws: connected": (r) => r && r.status === 101 });
    });
  }

  sleep(1);
}

// ── Итоговый отчёт ────────────────────────────────────────────
export function handleSummary(data) {
  const passed = data.metrics.errors.values.rate < 0.01;

  console.log("\n════════════════════════════════════");
  console.log(`  Load Test Result: ${passed ? "✅ PASSED" : "❌ FAILED"}`);
  console.log("════════════════════════════════════");
  console.log(`  HTTP requests:    ${data.metrics.http_reqs.values.count}`);
  console.log(`  Error rate:       ${(data.metrics.errors.values.rate * 100).toFixed(2)}%`);
  console.log(`  P95 latency:      ${data.metrics.http_req_duration.values["p(95)"].toFixed(0)}ms`);
  console.log(`  P99 latency:      ${data.metrics.http_req_duration.values["p(99)"].toFixed(0)}ms`);
  console.log(`  Messages sent:    ${data.metrics.messages_sent?.values?.count || 0}`);
  console.log("════════════════════════════════════\n");

  return {
    "k6/summary.json": JSON.stringify(data, null, 2),
    stdout: "\n",
  };
}