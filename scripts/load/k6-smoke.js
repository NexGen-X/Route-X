import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Skenario beban ringan Route-X: healthz, chat non-streaming, chat SSE + abort acak.
// Target: 20 VU selama 5 menit campuran, ambang: p95 < 800ms, error < 1%.
// Jalankan: k6 run -e BASE_URL=http://127.0.0.1:18080 -e API_KEY=sk_test_... scripts/load/k6-smoke.js

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:18080';
const KEY = __ENV.API_KEY || '';

export const options = {
  scenarios: {
    campuran: {
      executor: 'constant-vus',
      vus: 20,
      duration: '5m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<800'],
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
  // 1. healthz selalu hijau, tanpa auth.
  let r = http.get(BASE + '/healthz');
  check(r, { 'healthz 200': (x) => x.status === 200 });

  // 2. Chat non-streaming ke provider echo lokal.
  r = http.post(BASE + '/v1/chat/completions', BODY_NONSTREAM, {
    headers: headers(false),
    timeout: '30s',
  });
  const okNon = check(r, { 'chat 200': (x) => x.status === 200 });
  latNonStream.add(r.timings.duration);
  errRate.add(okNon ? 0 : 1);

  // 3. Chat SSE: separuh VU abort di tengah (batalkan setelah baca sebagian).
  // k6 tidak punya API abort mid-response, jadi simulasikan dengan timeout
  // pendek 2 detik pada separuh iterasi: server melihat klien pergi.
  const abort = Math.random() < 0.5;
  r = http.post(BASE + '/v1/chat/completions', BODY_STREAM, {
    headers: headers(true),
    timeout: abort ? '2s' : '30s',
  });
  if (abort) abortCount.add(1);
  const okSSE = check(r, {
    'sse 200 atau timeout-abort': (x) => x.status === 200 || x.status === 0,
  });
  latSSE.add(r.timings.duration);
  errRate.add(okSSE ? 0 : 1);

  sleep(1);
}
