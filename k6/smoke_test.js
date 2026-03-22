// k6/smoke_test.js
// Быстрая проверка после деплоя — 1 VU, 1 минута
// Запуск: k6 run k6/smoke_test.js

import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";

export const options = {
  vus: 1,
  duration: "30s",
  thresholds: {
    http_req_duration: ["p(99)<1000"],
    http_req_failed:   ["rate<0.001"],
  },
};

export default function () {
  // Health
  const health = http.get(`${BASE_URL}/health`);
  check(health, {
    "health: 200":  (r) => r.status === 200,
    "health: db ok": (r) => r.json("db") === true,
  });

  // Метрики доступны
  const metrics = http.get(`${BASE_URL}/metrics`);
  check(metrics, { "metrics: 200": (r) => r.status === 200 });

  // Register
  const reg = http.post(
    `${BASE_URL}/api/v1/auth/register`,
    JSON.stringify({ username: `smoke_${Date.now()}`, password: "Smoke123!" }),
    { headers: { "Content-Type": "application/json" } }
  );
  check(reg, { "register: 201": (r) => r.status === 201 });

  sleep(1);
}

export function handleSummary(data) {
  const ok = data.metrics.http_req_failed.values.rate < 0.001;
  console.log(`\nSmoke test: ${ok ? "✅ PASSED" : "❌ FAILED"}\n`);
  return { stdout: "\n" };
}