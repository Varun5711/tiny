import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomString } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const urlCreationErrors = new Counter('url_creation_errors');
const redirectErrors = new Counter('redirect_errors');
const urlCreationDuration = new Trend('url_creation_duration');
const redirectDuration = new Trend('redirect_duration');
const successRate = new Rate('success_rate');

const API_GATEWAY_URL = __ENV.API_GATEWAY_URL || 'http://localhost:8080';
const REDIRECT_URL = __ENV.REDIRECT_SERVICE_URL || 'http://localhost:8081';

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '30s',
      startTime: '0s',
      tags: { test_type: 'smoke' },
    },
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 50 },   
        { duration: '3m', target: 50 }, 
        { duration: '1m', target: 100 },  
        { duration: '3m', target: 100 },
        { duration: '1m', target: 0 },
      ],
      startTime: '35s',
      tags: { test_type: 'load' },
    },
    // Stress test - beyond normal load
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 200 },  // Ramp up to 200 users
        { duration: '5m', target: 200 },  // Stay at 200 users
        { duration: '2m', target: 300 },  // Ramp up to 300 users
        { duration: '5m', target: 300 },  // Stay at 300 users
        { duration: '2m', target: 0 },    // Ramp down
      ],
      startTime: '10m',
      tags: { test_type: 'stress' },
    },
    spike: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '10s', target: 500 }, // Sudden spike
        { duration: '1m', target: 500 },  // Stay at spike
        { duration: '10s', target: 0 },   // Quick drop
      ],
      startTime: '26m',
      tags: { test_type: 'spike' },
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'], // 95% of requests under 500ms
    http_req_failed: ['rate<0.01'],                  // Error rate under 1%
    url_creation_duration: ['p(95)<1000'],           // URL creation under 1s
    redirect_duration: ['p(95)<100'],                // Redirects under 100ms
    success_rate: ['rate>0.99'],                     // 99% success rate
  },
};

// Shared state for created URLs
let createdShortCodes = [];
let authToken = null;

// Setup function - runs once before tests
export function setup() {
  // Register and login to get auth token
  const email = `loadtest-${randomString(8)}@example.com`;
  const password = 'loadtest123';

  const registerRes = http.post(
    `${API_GATEWAY_URL}/api/auth/register`,
    JSON.stringify({
      email: email,
      password: password,
      name: 'Load Test User',
    }),
    { headers: { 'Content-Type': 'application/json' } }
  );

  if (registerRes.status === 201 || registerRes.status === 200) {
    const body = JSON.parse(registerRes.body);
    return { token: body.token, email: email };
  }

  const loginRes = http.post(
    `${API_GATEWAY_URL}/api/auth/login`,
    JSON.stringify({ email: email, password: password }),
    { headers: { 'Content-Type': 'application/json' } }
  );

  if (loginRes.status === 200) {
    const body = JSON.parse(loginRes.body);
    return { token: body.token, email: email };
  }

  return { token: null, email: null };
}

// Main test function
export default function (data) {
  const headers = {
    'Content-Type': 'application/json',
  };

  if (data.token) {
    headers['Authorization'] = `Bearer ${data.token}`;
  }

  group('Health Check', function () {
    const res = http.get(`${API_GATEWAY_URL}/health`);
    check(res, {
      'health check status is 200': (r) => r.status === 200,
    });
  });

  group('Create Short URL', function () {
    const startTime = Date.now();
    const payload = JSON.stringify({
      long_url: `https://example.com/test-${randomString(10)}-${Date.now()}`,
    });

    const res = http.post(`${API_GATEWAY_URL}/api/urls`, payload, { headers });

    urlCreationDuration.add(Date.now() - startTime);

    const success = check(res, {
      'create URL status is 201 or 200': (r) => r.status === 201 || r.status === 200,
      'response has short_code': (r) => {
        try {
          const body = JSON.parse(r.body);
          return body.short_code !== undefined;
        } catch {
          return false;
        }
      },
    });

    if (success) {
      successRate.add(1);
      try {
        const body = JSON.parse(res.body);
        if (body.short_code) {
          createdShortCodes.push(body.short_code);
          // Keep only last 1000 codes to avoid memory issues
          if (createdShortCodes.length > 1000) {
            createdShortCodes.shift();
          }
        }
      } catch {}
    } else {
      successRate.add(0);
      urlCreationErrors.add(1);
    }
  });

  group('Redirect URL', function () {
    // Use a random existing short code or generate one
    let shortCode;
    if (createdShortCodes.length > 0) {
      shortCode = createdShortCodes[Math.floor(Math.random() * createdShortCodes.length)];
    } else {
      shortCode = 'test123'; // Fallback
    }

    const startTime = Date.now();
    const res = http.get(`${REDIRECT_URL}/${shortCode}`, {
      redirects: 0, // Don't follow redirects
    });

    redirectDuration.add(Date.now() - startTime);

    const success = check(res, {
      'redirect status is 302 or 301': (r) => r.status === 302 || r.status === 301,
      'has location header': (r) => r.headers['Location'] !== undefined,
    });

    if (success) {
      successRate.add(1);
    } else {
      successRate.add(0);
      if (res.status !== 404) {
        // 404 is expected for non-existent codes
        redirectErrors.add(1);
      }
    }
  });

  group('Create Custom URL', function () {
    const customAlias = `k6-${randomString(8)}-${Date.now()}`;
    const payload = JSON.stringify({
      alias: customAlias,
      long_url: `https://example.com/custom-${randomString(10)}`,
    });

    const res = http.post(`${API_GATEWAY_URL}/api/urls/custom`, payload, { headers });

    const success = check(res, {
      'custom URL status is 201 or 200': (r) => r.status === 201 || r.status === 200,
      'custom URL has correct alias': (r) => {
        try {
          const body = JSON.parse(r.body);
          return body.short_code === customAlias;
        } catch {
          return false;
        }
      },
    });

    if (success) {
      successRate.add(1);
      createdShortCodes.push(customAlias);
    } else {
      successRate.add(0);
    }
  });

  group('List URLs', function () {
    const res = http.get(`${API_GATEWAY_URL}/api/urls`, { headers });

    check(res, {
      'list URLs status is 200': (r) => r.status === 200,
      'response has urls array': (r) => {
        try {
          const body = JSON.parse(r.body);
          return Array.isArray(body.urls);
        } catch {
          return false;
        }
      },
    });
  });

  sleep(Math.random() * 2 + 0.5); // Random sleep between 0.5-2.5 seconds
}

// Teardown function - runs once after all tests
export function teardown(data) {
  console.log(`Test completed. Created ${createdShortCodes.length} short codes.`);
}
