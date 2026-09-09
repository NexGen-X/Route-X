import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Skenario beban BERAT Route-X: ramp 0->200 VU, tahan, turun.
// Fase: naik 2 mnt, tahan 200 VU 5 mnt, turun 1 mnt. Total ~8 menit.
// Ambang: p95 < 2000ms (longgar vs smoke karena 10x VU), error < 5%.
// Jalankan: k6 run -e BASE_URL=http://127.0.0.1:18080 -e API_KEY=sk_test_... scripts/load/k6-berat.js

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:18080';
const KEY = __ENV.API_KEY || '';

export const options = {
  scenarios: {
    berat: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 200 },
        { duration: '5m', target: 200 },
        { duration: '1m', target: 0 },
      ],
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<2000'],
  },
};

const errRate = new Rate('rx_error');
const latNonStream = new Trend('rx_chat_nonstream_ms');
const latSSE = new Trend('rx_chat_sse_ms');
const abortCount = new Counter('rx_sse_abort');

const BODY_NONSTREAM = JSON.stringify({
  model: 'gpt-5',
  messages: [{ role: 'user', content: 'jawab dengan satu kata: siap' }],
});
const BODY_STREAM = JSON.stringify({
  model: 'gpt-5',
  messages: [{ role: 'user', content: 'hitung satu sampai tiga' }],
  stream: true,
});

function headers(stream) {
  const h = {
    'Content-Type': 'application/json',
    Authorization: 'Bearer ' + KEY,
  };
  if (stream) h['Accept'] = 'text/event-stream';
  return h;
}

export default function () {
  let r = http.get(BASE + '/healthz');
  check(r, { 'healthz 200': (x) => x.status === 200 });

  r = http.post(BASE + '/v1/chat/completions', BODY_NONSTREAM, {
    headers: headers(false),
    timeout: '60s',
  });
  const okNon = check(r, { 'chat 200': (x) => x.status === 200 });
  latNonStream.add(r.timings.duration);
  errRate.add(okNon ? 0 : 1);

  // 30% SSE abort via timeout pendek 2 detik.
  const abort = Math.random() < 0.3;
  r = http.post(BASE + '/v1/chat/completions', BODY_STREAM, {
    headers: headers(true),
    timeout: abort ? '2s' : '60s',
  });
  if (abort) abortCount.add(1);
  const okSSE = check(r, {
    'sse 200 atau timeout-abort': (x) => x.status === 200 || x.status === 0,
  });
  latSSE.add(r.timings.duration);
  errRate.add(okSSE ? 0 : 1);

  sleep(0.5);
}
