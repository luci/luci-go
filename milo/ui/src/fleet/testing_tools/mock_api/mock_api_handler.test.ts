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

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import sampleAndroidDevices from './data/android_devices.json';
import sampleChromeosDevices from './data/chromeos_devices.json';
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
    expect(reset.devices[0].id).toBe(sampleChromeosDevices[0].id);
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
    expect(parsed.total).toBe(
      sampleChromeosDevices.length + sampleAndroidDevices.length,
    );
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

  it('evaluates AIP-160 AST filters (including nested parentheses and quoted AND/OR) on ListDevices and CountDevices', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    const listRes = await fetch('/prpc/fleetconsole.FleetConsole/ListDevices', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        filter:
          '(labels."dut_state" = "needs_repair") OR (labels."dut_state" = "repair_failed")',
      }),
    });
    const listJson = JSON.parse((await listRes.text()).replace(")]}'\n", ''));
    expect(listJson.devices.length).toBe(3);

    const countRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/CountDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filter: [
            'labels."label-pool" = "labstation_tryjob"',
            'labels."label-pool" = "labstation_main"',
            'labels."label-pool" = "labstation_canary"',
          ].join(' OR '),
        }),
      },
    );
    const countJson = JSON.parse((await countRes.text()).replace(")]}'\n", ''));
    expect(countJson.total).toBe(3);
    expect(countJson.deviceState.repairFailed).toBe(1);

    expect(
      FleetConsoleMockAPI.matchesAip160Filter(
        {
          state: 'DEVICE_STATE_AVAILABLE',
          deviceSpec: {
            labels: {
              notes: { values: ['fish AND chips'] },
              dut_state: { values: ['ready'] },
            },
          },
        },
        '(labels."notes" = "fish AND chips") AND (labels."dut_state" = "ready" OR labels."dut_state" = "needs_repair")',
      ),
    ).toBe(true);

    expect(
      FleetConsoleMockAPI.matchesAip160Filter(
        {
          state: 'DEVICE_STATE_AVAILABLE',
          deviceSpec: {
            labels: {
              notes: { values: ['other'] },
              dut_state: { values: ['needs_repair'] },
            },
          },
        },
        '(labels."notes" = "fish AND chips" AND labels."dut_state" = "ready") OR labels."dut_state" = "needs_repair"',
      ),
    ).toBe(true);

    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  it('returns dynamic Fleet Health trends by grouping and Swarming/UFS fixtures for Device Details', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    const overallRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/GetFleetAvailabilityTrends',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          grouping: 'GROUP_BY_OVERALL',
          filter: 'labels."label-pool" = "mock-pool-dev"',
        }),
      },
    );
    const overallJson = JSON.parse(
      (await overallRes.text()).replace(")]}'\n", ''),
    );
    expect(overallJson.series).toHaveLength(1);
    expect(overallJson.series[0].name).toBe('Overall');
    expect(overallJson.series[0].points.length).toBeGreaterThan(24);
    expect(typeof overallJson.series[0].points[0].value).toBe('number');
    expect(overallJson.series[0].points[0].value).toBeGreaterThan(0.5);

    const modelRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/GetFleetAvailabilityTrends',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ grouping: 'GROUP_BY_MODEL' }),
      },
    );
    const modelJson = JSON.parse((await modelRes.text()).replace(")]}'\n", ''));
    expect(modelJson.series.length).toBeGreaterThan(1);
    expect(modelJson.series[0].name).toBe('sim-model-nine');
    expect(modelJson.series[0].points[0].value).toBeGreaterThan(0.5);

    const singleDeviceRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          pageSize: 1,
          filter: 'id = "demo-cros-r2-rk1-h2"',
        }),
      },
    );
    const singleDeviceJson = JSON.parse(
      (await singleDeviceRes.text()).replace(")]}'\n", ''),
    );
    expect(singleDeviceJson.devices).toHaveLength(1);
    expect(singleDeviceJson.devices[0].id).toBe('demo-cros-r2-rk1-h2');

    const botTasksRes = await fetch('/prpc/swarming.v2.Bots/ListBotTasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ botId: 'crossk-demo-cros-r2-rk1-h2' }),
    });
    const botTasksJson = JSON.parse(
      (await botTasksRes.text()).replace(")]}'\n", ''),
    );
    expect(botTasksJson.items.length).toBeGreaterThanOrEqual(5);

    const ufsRes = await fetch(
      '/prpc/unifiedfleet.api.v1.rpc.Fleet/GetMachineLSE',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'machineLSEs/chromeos-device-02' }),
      },
    );
    const ufsJson = JSON.parse((await ufsRes.text()).replace(")]}'\n", ''));
    const decodedLse = MachineLSE.fromJSON(ufsJson);
    expect(decodedLse.hostname).toBe('chromeos-device-02');
    expect(
      decodedLse.chromeosMachineLse?.deviceLse?.dut?.peripherals?.servo
        ?.servoHostname,
    ).toBe('chromeos-device-02-labstation1');

    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  it('supports cross-fixture drill-down, filtered trends, slice mutations, and Swarming pagination', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();

    // 1. Cross-fixture drill-down: ListDevices & GetDeviceDimensions include mock-board-epsilon / sim-model-nine / mock-pool-dev
    const bryaDevicesRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filter: 'labels."label-board" = "mock-board-epsilon"',
        }),
      },
    );
    const bryaDevicesJson = JSON.parse(
      (await bryaDevicesRes.text()).replace(")]}'\n", ''),
    );
    expect(bryaDevicesJson.devices.length).toBeGreaterThanOrEqual(2);

    const quotaDevicesRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filter: 'labels."label-pool" = "mock-pool-dev"',
        }),
      },
    );
    const quotaDevicesJson = JSON.parse(
      (await quotaDevicesRes.text()).replace(")]}'\n", ''),
    );
    expect(quotaDevicesJson.devices.length).toBeGreaterThanOrEqual(2);

    const dimsRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/GetDeviceDimensions',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      },
    );
    const dimsJson = JSON.parse((await dimsRes.text()).replace(")]}'\n", ''));
    expect(dimsJson.labels['label-board'].values).toContain(
      'mock-board-epsilon',
    );
    expect(dimsJson.labels['label-model'].values).toContain('sim-model-nine');
    expect(dimsJson.labels['label-pool'].values).toContain('mock-pool-dev');

    // 2. GetFleetAvailabilityTrends honors filter in GROUP_BY_MODEL and GROUP_BY_POOL and emits numeric metricType
    const filteredModelRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/GetFleetAvailabilityTrends',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          grouping: 'GROUP_BY_MODEL',
          filter: 'labels."label-model" = "sim-model-nine"',
        }),
      },
    );
    const filteredModelJson = JSON.parse(
      (await filteredModelRes.text()).replace(")]}'\n", ''),
    );
    expect(filteredModelJson.metricType).toBe(1);
    expect(filteredModelJson.series).toHaveLength(1);
    expect(filteredModelJson.series[0].name).toBe('sim-model-nine');
    expect(Object.keys(filteredModelJson.series[0].points[0]).sort()).toEqual([
      'timestamp',
      'value',
    ]);

    const filteredPoolRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/GetFleetAvailabilityTrends',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          grouping: 'GROUP_BY_POOL',
          filter: 'labels."label-pool" = "mock-pool-dev"',
        }),
      },
    );
    const filteredPoolJson = JSON.parse(
      (await filteredPoolRes.text()).replace(")]}'\n", ''),
    );
    expect(filteredPoolJson.metricType).toBe(2);
    expect(filteredPoolJson.series).toHaveLength(1);
    expect(filteredPoolJson.series[0].name).toBe('mock-pool-dev');

    // 3. Stateful Health Slice mutations persist into ListHealthSlices
    const createSliceRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/CreateHealthSlice',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          healthSlice: {
            name: 'Alpha Fleet',
            filter: 'labels."label-board" = "demo-board-alpha"',
          },
        }),
      },
    );
    const createdSlice = JSON.parse(
      (await createSliceRes.text()).replace(")]}'\n", ''),
    ).healthSlice;
    expect(createdSlice.name).toBe('Alpha Fleet');

    const updateSliceRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/UpdateHealthSlice',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          healthSlice: {
            id: createdSlice.id,
            name: 'Alpha Canary',
            filter: 'labels."label-board" = "demo-board-alpha"',
          },
        }),
      },
    );
    const updatedSlice = JSON.parse(
      (await updateSliceRes.text()).replace(")]}'\n", ''),
    ).healthSlice;
    expect(updatedSlice.name).toBe('Alpha Canary');

    const listAfterUpdateRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListHealthSlices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      },
    );
    const listAfterUpdateJson = JSON.parse(
      (await listAfterUpdateRes.text()).replace(")]}'\n", ''),
    );
    expect(
      listAfterUpdateJson.healthSlices.some(
        (s: { id: string; name: string }) =>
          s.id === createdSlice.id && s.name === 'Alpha Canary',
      ),
    ).toBe(true);

    await fetch('/prpc/fleetconsole.FleetConsole/DeleteHealthSlice', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: createdSlice.id }),
    });
    const listAfterDeleteRes = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListHealthSlices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      },
    );
    const listAfterDeleteJson = JSON.parse(
      (await listAfterDeleteRes.text()).replace(")]}'\n", ''),
    );
    expect(
      listAfterDeleteJson.healthSlices.some(
        (s: { id: string }) => s.id === createdSlice.id,
      ),
    ).toBe(false);

    // 4. Swarming ListBotTasks pagination with limit & cursor
    const page1TasksRes = await fetch('/prpc/swarming.v2.Bots/ListBotTasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        botId: 'crossk-demo-cros-r2-rk1-h2',
        limit: 3,
      }),
    });
    const page1TasksJson = JSON.parse(
      (await page1TasksRes.text()).replace(")]}'\n", ''),
    );
    expect(page1TasksJson.items).toHaveLength(3);
    expect(page1TasksJson.cursor).toBe('token-3');

    const page2TasksRes = await fetch('/prpc/swarming.v2.Bots/ListBotTasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        botId: 'crossk-demo-cros-r2-rk1-h2',
        limit: 3,
        cursor: page1TasksJson.cursor,
      }),
    });
    const page2TasksJson = JSON.parse(
      (await page2TasksRes.text()).replace(")]}'\n", ''),
    );
    expect(page2TasksJson.items).toHaveLength(3);
    expect(page2TasksJson.items[0].taskId).not.toBe(
      page1TasksJson.items[0].taskId,
    );

    // 5. Dynamic GetBuild output.properties & summaryMarkdown for passed vs failed builds
    const failedBuildRes = await fetch('/prpc/buildbucket.v2.Builds/GetBuild', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: '8741928374918273603' }),
    });
    const failedBuildJson = JSON.parse(
      (await failedBuildRes.text()).replace(")]}'\n", ''),
    );
    expect(failedBuildJson.status).toBe(20);
    expect(
      failedBuildJson.output.properties.CompressedResult.chromeos_version,
    ).toBe('16180.0.0');
    expect(failedBuildJson.summaryMarkdown).toContain(
      'tast.video-decode-accel-h264',
    );

    FleetConsoleMockAPI.disableBrowserInterceptor();
  });
});
