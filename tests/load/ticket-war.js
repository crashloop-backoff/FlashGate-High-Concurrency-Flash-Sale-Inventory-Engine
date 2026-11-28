import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const EVENT_ID = __ENV.EVENT_ID;

if (!EVENT_ID) {
  throw new Error("EVENT_ID environment variable is required (e.g. -e EVENT_ID=<uuid>)");
}

const reserve201 = new Counter("flashgate_k6_reserve_201");
const reserve200Replay = new Counter("flashgate_k6_reserve_200_replay");
const reserve409Conflict = new Counter("flashgate_k6_reserve_409");

export const options = {
  scenarios: {
    ticket_war: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "5s", target: 20 },  // Warm-up to 20 VUs
        { duration: "30s", target: 200 }, // Spike to 200 VUs
        { duration: "10s", target: 0 },   // Ramp-down to 0 VUs
      ],
      gracefulRampDown: "5s",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
  },
};

// Per-VU state to simulate 10% network retry with an identical Idempotency-Key.
let lastIdempotencyKey = null;
let lastUserID = null;

export default function () {
  const isRetry = lastIdempotencyKey !== null && Math.random() < 0.10;
  const userID = isRetry ? lastUserID : `vu-${__VU}`;
  const idemKey = isRetry
    ? lastIdempotencyKey
    : `k6-${EVENT_ID}-${__VU}-${__ITER}-${Date.now()}`;

  lastIdempotencyKey = idemKey;
  lastUserID = userID;

  const reserveRes = http.post(
    `${BASE_URL}/v1/orders`,
    JSON.stringify({
      event_id: EVENT_ID,
      quantity: 1,
    }),
    {
      headers: {
        "Content-Type": "application/json",
        "X-User-ID": userID,
        "Idempotency-Key": idemKey,
      },
      expectedStatuses: http.expectedStatuses(200, 201, 409),
      tags: { name: "POST /v1/orders" },
    }
  );

  check(reserveRes, {
    "reserve status is 201, 200, or 409": (r) =>
      r.status === 201 || r.status === 200 || r.status === 409,
  });

  if (reserveRes.status === 201) {
    reserve201.add(1);
  } else if (reserveRes.status === 200) {
    reserve200Replay.add(1);
    return;
  } else if (reserveRes.status === 409) {
    reserve409Conflict.add(1);
    return;
  } else {
    return;
  }

  let orderID = "";
  try {
    const body = reserveRes.json();
    orderID = body.id || "";
  } catch (_) {
    return;
  }
  if (!orderID) {
    return;
  }

  // Simulate order lifecycle: 60% pay, 10% cancel, 30% left RESERVED for sweeper.
  const roll = Math.random();
  if (roll < 0.60) {
    const payRes = http.post(`${BASE_URL}/v1/orders/${orderID}/pay`, null, {
      expectedStatuses: http.expectedStatuses(200, 409),
      tags: { name: "POST /v1/orders/{order_id}/pay" },
    });
    check(payRes, {
      "pay status is 200 or 409": (r) => r.status === 200 || r.status === 409,
    });
  } else if (roll < 0.70) {
    const cancelRes = http.post(`${BASE_URL}/v1/orders/${orderID}/cancel`, null, {
      expectedStatuses: http.expectedStatuses(200, 409),
      tags: { name: "POST /v1/orders/{order_id}/cancel" },
    });
    check(cancelRes, {
      "cancel status is 200 or 409": (r) => r.status === 200 || r.status === 409,
    });
  }
}
