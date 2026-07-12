import { test, expect, APIRequestContext } from '@playwright/test';

const SOURCE_API = process.env.SOURCE_API_URL ?? 'http://localhost:8081';
const TARGET_API = process.env.TARGET_API_URL ?? 'http://localhost:8082';
const API_TOKEN = process.env.API_TOKEN ?? 'local-dev-token';
const AUTH = { Authorization: `Bearer ${API_TOKEN}` };

// ターゲット側に注文が反映されるまでポーリングする
async function waitForReplication(
  request: APIRequestContext,
  orderId: string,
  timeoutMs = 60_000,
): Promise<Record<string, string>> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await request.get(`${TARGET_API}/orders/${orderId}`, { headers: AUTH });
    if (res.status() === 200) {
      return (await res.json()) as Record<string, string>;
    }
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`注文 ${orderId} が時間内に反映されませんでした`);
}

test.describe('CDCパイプライン', () => {
  test('両APIのヘルスチェックが通る', async ({ request }) => {
    const source = await request.get(`${SOURCE_API}/healthz`);
    const target = await request.get(`${TARGET_API}/healthz`);
    expect(source.status()).toBe(200);
    expect(target.status()).toBe(200);
  });

  test('注文作成がCDC経由でターゲットへ反映される', async ({ request }) => {
    const res = await request.post(`${SOURCE_API}/orders`, {
      headers: AUTH,
      data: { customer_id: 'e2e-cust-1', amount: '2480.00' },
    });
    expect(res.status()).toBe(201);
    const order = (await res.json()) as Record<string, string>;
    expect(order.id).toBeTruthy();

    const replica = await waitForReplication(request, order.id);
    expect(replica.order_id).toBe(order.id);
    expect(replica.customer_id).toBe('e2e-cust-1');
    expect(replica.amount).toBe('2480.00');
    expect(replica.status).toBe('created');
    expect(replica.event_id).toBeTruthy();
  });

  test('同一顧客の連続注文がすべて反映される', async ({ request }) => {
    const ids: string[] = [];
    for (let i = 0; i < 3; i++) {
      const res = await request.post(`${SOURCE_API}/orders`, {
        headers: AUTH,
        data: { customer_id: 'e2e-cust-2', amount: `${1000 + i}.00` },
      });
      expect(res.status()).toBe(201);
      const order = (await res.json()) as Record<string, string>;
      ids.push(order.id);
    }
    for (const id of ids) {
      const replica = await waitForReplication(request, id);
      expect(replica.order_id).toBe(id);
    }
  });

  test('同じべき等キーの重複配送では金額が上書きされない', async ({ request }) => {
    const res = await request.post(`${SOURCE_API}/orders`, {
      headers: AUTH,
      data: { customer_id: 'e2e-cust-3', amount: '5000.00' },
    });
    expect(res.status()).toBe(201);
    const order = (await res.json()) as Record<string, string>;
    const replica = await waitForReplication(request, order.id);

    // 配送Lambdaの再送を模して、同じイベントIDで金額を変えて直接再送する
    const dup = await request.post(`${TARGET_API}/orders/replicate`, {
      headers: { ...AUTH, 'X-Idempotency-Key': replica.event_id },
      data: {
        order_id: order.id,
        customer_id: 'e2e-cust-3',
        amount: '1.00',
        status: 'created',
        seq: 999999,
      },
    });
    expect(dup.status()).toBe(200);

    const after = await request.get(`${TARGET_API}/orders/${order.id}`, { headers: AUTH });
    expect(after.status()).toBe(200);
    const body = (await after.json()) as Record<string, string>;
    expect(body.amount).toBe('5000.00');
  });

  test('不正な注文入力は400で拒否される', async ({ request }) => {
    const missing = await request.post(`${SOURCE_API}/orders`, {
      headers: AUTH,
      data: { customer_id: '', amount: '100' },
    });
    expect(missing.status()).toBe(400);

    const badAmount = await request.post(`${SOURCE_API}/orders`, {
      headers: AUTH,
      data: { customer_id: 'e2e-cust-4', amount: 'abc' },
    });
    expect(badAmount.status()).toBe(400);
  });

  test('存在しない注文の照会は404を返す', async ({ request }) => {
    const res = await request.get(`${TARGET_API}/orders/00000000-0000-4000-8000-000000000000`, {
      headers: AUTH,
    });
    expect(res.status()).toBe(404);
  });

  test('認証トークンなしのアクセスは401で拒否される', async ({ request }) => {
    const source = await request.post(`${SOURCE_API}/orders`, {
      data: { customer_id: 'e2e-cust-5', amount: '100' },
    });
    expect(source.status()).toBe(401);
    const target = await request.get(`${TARGET_API}/orders/xxx`);
    expect(target.status()).toBe(401);
  });
});
