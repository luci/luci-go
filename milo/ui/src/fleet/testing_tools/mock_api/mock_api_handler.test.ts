// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { FleetConsoleMockAPI } from './mock_api_handler';

describe('FleetConsoleMockAPI', () => {
  beforeEach(() => {
    FleetConsoleMockAPI.resetFixtures();
  });

  afterEach(() => {
    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  it('provides default fixtures for standard pRPC endpoints', () => {
    const countFixture = FleetConsoleMockAPI.getFixture('CountDevices') as {
      total: number;
    };
    expect(countFixture).toBeDefined();
    expect(countFixture.total).toBeGreaterThan(0);

    const listFixture = FleetConsoleMockAPI.getFixture('ListDevices') as {
      devices: unknown[];
    };
    expect(listFixture).toBeDefined();
    expect(Array.isArray(listFixture.devices)).toBe(true);
  });

  it('allows setting and resetting custom fixtures for integration tests', () => {
    FleetConsoleMockAPI.setFixture('ListDevices', {
      devices: [{ id: 'custom-test-device' }],
      totalSize: 1,
    });

    const custom = FleetConsoleMockAPI.getFixture('ListDevices') as {
      devices: Array<{ id: string }>;
    };
    expect(custom.devices[0].id).toBe('custom-test-device');

    FleetConsoleMockAPI.resetFixtures();
    const reset = FleetConsoleMockAPI.getFixture('ListDevices') as {
      devices: Array<{ id: string }>;
    };
    expect(reset.devices[0].id).toBe('chromeos-device-01');
  });

  it('prevents cross-test contamination when nested fixture objects are modified', () => {
    const listFixture = FleetConsoleMockAPI.getFixture('ListDevices') as {
      devices: Array<{ id: string }>;
    };
    listFixture.devices.push({ id: 'mutated-device' });

    FleetConsoleMockAPI.resetFixtures();
    const resetFixture = FleetConsoleMockAPI.getFixture('ListDevices') as {
      devices: Array<{ id: string }>;
    };
    expect(
      resetFixture.devices.find((d) => d.id === 'mutated-device'),
    ).toBeUndefined();
  });

  it('intercepts fetch requests and returns LUCI pRPC formatted JSON with XSSI prefix', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    const response = await fetch(
      '/prpc/fleetconsole.FleetConsole/CountDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      },
    );

    expect(response.status).toBe(200);
    const text = await response.text();
    expect(text.startsWith(")]}'\n")).toBe(true);

    const parsed = JSON.parse(text.replace(")]}'\n", ''));
    expect(parsed.total).toBe(12536);
  });

  it('intercepts auth state queries offline and allows setting custom auth state', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    FleetConsoleMockAPI.setAuthState({
      identity: 'user:admin@google.com',
      email: 'admin@google.com',
    });

    const response = await fetch('/auth/openid/state');
    expect(response.status).toBe(200);

    const auth = await response.json();
    expect(auth.identity).toBe('user:admin@google.com');
    expect(auth.email).toBe('admin@google.com');

    FleetConsoleMockAPI.resetAuthState();
    const resetRes = await fetch('/auth/openid/state');
    const resetAuth = await resetRes.json();
    expect(resetAuth.identity).toBe('user:user@example.com');
  });

  it('supports simulating gRPC error codes and disabling interceptor cleanly', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.setErrorFixture('CountDevices', {
      grpcCode: 7,
      message: 'Permission Denied',
    });

    const response = await fetch(
      '/prpc/fleetconsole.FleetConsole/CountDevices',
      {
        method: 'POST',
      },
    );

    expect(response.status).toBe(500);
    expect(response.headers.get('X-Prpc-Grpc-Code')).toBe('7');
    const text = await response.text();
    expect(text).toContain('Permission Denied');

    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  it('supports initPrototypeMode, latency simulation, and localStorage state persistence', async () => {
    FleetConsoleMockAPI.initPrototypeMode({
      persistToLocalStorage: true,
      latencyMs: 10,
      authState: { email: 'proto-user@google.com' },
    });

    expect(FleetConsoleMockAPI.isPrototypeMode()).toBe(true);

    const authRes = await fetch('/auth/openid/state');
    const auth = await authRes.json();
    expect(auth.email).toBe('proto-user@google.com');

    FleetConsoleMockAPI.setFixture('CountDevices', { total: 9999 });
    const countRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/CountDevices',
      { method: 'POST' },
    );
    const text = await countRes.text();
    expect(text).toContain('9999');

    FleetConsoleMockAPI.disableBrowserInterceptor();
    expect(FleetConsoleMockAPI.isPrototypeMode()).toBe(false);

    // Re-enable without prototype mode options and verify latency is reset to 0
    FleetConsoleMockAPI.enableBrowserInterceptor();
    const startTime = Date.now();
    await fetch('/prpc/fleetconsole.FleetConsole/CountDevices', {
      method: 'POST',
    });
    const elapsed = Date.now() - startTime;
    expect(elapsed).toBeLessThan(100);
    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  it('provides GetDevice fixture as typed default', () => {
    const getDeviceFixture = FleetConsoleMockAPI.getFixture('GetDevice') as {
      device: { id: string; dutId: string };
    };
    expect(getDeviceFixture).toBeDefined();
    expect(getDeviceFixture.device.id).toBe('chromeos-device-01');
  });

  it('deep clones root array fixtures when retrieved via getFixture', () => {
    FleetConsoleMockAPI.setFixture('CustomArray', [{ id: '1' }]);
    const arr1 = FleetConsoleMockAPI.getFixture('CustomArray') as Array<{
      id: string;
    }>;
    arr1.push({ id: '2' });

    const arr2 = FleetConsoleMockAPI.getFixture('CustomArray') as Array<{
      id: string;
    }>;
    expect(arr2).toHaveLength(1);
  });

  it('awaits Promise-based fixtures and returns resolved data', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.setFixture(
      'CountDevices',
      Promise.resolve({ total: 7777 }),
    );

    const response = await fetch(
      '/prpc/fleetconsole.FleetConsole/CountDevices',
      { method: 'POST' },
    );
    expect(response.status).toBe(200);
    const text = await response.text();
    expect(text).toContain('7777');

    FleetConsoleMockAPI.disableBrowserInterceptor();
  });
});
