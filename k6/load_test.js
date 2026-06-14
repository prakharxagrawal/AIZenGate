import http from 'k6/http';
import { check, sleep } from 'k6';

// k6 Options configuration
export const options = {
  stages: [
    { duration: '10s', target: 20 }, // ramp up to 20 virtual users
    { duration: '15s', target: 20 }, // stay at 20 users
    { duration: '5s', target: 0 },  // ramp down
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'], // less than 1% request failures (excluding rate limit 429s if handled in test checks)
    http_req_duration: ['p(95)<500'], // 95% of requests must complete below 500ms
  },
};

const BASE_URL = __ENV.ZENGATE_URL || 'http://localhost:8080';

// Mock tokens generated beforehand
const PREMIUM_TOKEN = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJsb2FkX3Rlc3RfcHJlbWl1bSIsInRpZXIiOiJwcmVtaXVtIn0.1ArHwhUFpe2hM_83TqV8Xe7Pd_gtxKsJC54ttYUittM';
const BASIC_TOKEN = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJsb2FkX3Rlc3RfYmFzaWMiLCJ0aWVyIjoiYmFzaWMifQ.6vO3HtCC0P34hWb9SUJqVuM7PKCxpMB7_asABvs-q2Y';

export default function () {
  // Select user profile randomly: premium, basic, or anonymous
  const rand = Math.random();
  let headers = {};
  let expectedTiers = 'anonymous';

  if (rand < 0.3) {
    // 30% Premium requests
    headers['Authorization'] = `Bearer ${PREMIUM_TOKEN}`;
    expectedTiers = 'premium';
  } else if (rand < 0.7) {
    // 40% Basic requests
    headers['Authorization'] = `Bearer ${BASIC_TOKEN}`;
    expectedTiers = 'basic';
  } // 30% Anonymous requests (no Authorization header)

  // 1. Health check call (unauthenticated)
  let healthRes = http.get(`${BASE_URL}/health`);
  check(healthRes, {
    'health check status is 200': (r) => r.status === 200,
  });

  sleep(0.1);

  // 2. Proxy request (through rate limit and authentication check)
  let proxyRes = http.get(`${BASE_URL}/anything`, { headers: headers });
  check(proxyRes, {
    'proxy status is 200 or 429 (rate-limited)': (r) => r.status === 200 || r.status === 429,
  });

  sleep(0.2);
}
