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

  it('intercepts auth state queries offline without network errors', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    const response = await fetch('/auth/openid/state');
    expect(response.status).toBe(200);

    const auth = await response.json();
    expect(auth.identity).toBe('user:zhangtiff@google.com');
  });
});
