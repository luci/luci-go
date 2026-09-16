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

import { FleetConsoleClientImpl } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import sampleChromeosDevices from './data/chromeos_devices.json';
import { FleetConsoleMockAPI } from './mock_api_handler';

describe('FleetConsoleMockAPI Schema Synchronization Guardrail', () => {
  it('has registered mock fixtures for all methods in FleetConsoleClientImpl', () => {
    // Collect all RPC methods implemented on FleetConsoleClientImpl prototype
    const clientPrototype = FleetConsoleClientImpl.prototype;
    const clientMethods = Object.getOwnPropertyNames(clientPrototype).filter(
      (prop) =>
        prop !== 'constructor' &&
        typeof (clientPrototype as unknown as Record<string, unknown>)[prop] ===
          'function',
    );

    expect(clientMethods.length).toBeGreaterThan(0);

    const missingFixtures: string[] = [];
    for (const method of clientMethods) {
      const fixture = FleetConsoleMockAPI.getFixture(method);
      if (fixture === undefined) {
        missingFixtures.push(method);
      }
    }

    expect(missingFixtures).toEqual([]);
  });

  it('evaluates in-memory AIP-160 filters dynamically during intercepted calls', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    const targetDevice = sampleChromeosDevices[0];
    const targetBoard = targetDevice.deviceSpec.labels['label-board'].values[0];

    // Query with filter for first board
    const filteredResponse = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ filter: `board = "${targetBoard}"` }),
      },
    );

    expect(filteredResponse.status).toBe(200);
    const text = await filteredResponse.text();
    const parsed = JSON.parse(text.replace(")]}'\n", ''));
    expect(parsed.devices.length).toBeGreaterThan(0);
    expect(parsed.devices[0].id).toBe(targetDevice.id);
  });

  it('paginates mock item collections with token offset handling', async () => {
    FleetConsoleMockAPI.enableBrowserInterceptor();

    const pagedResponse = await fetch(
      '/prpc/fleetconsole.FleetConsole/ListDevices',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pageSize: 1 }),
      },
    );

    const text = await pagedResponse.text();
    const parsed = JSON.parse(text.replace(")]}'\n", ''));
    expect(parsed.devices).toHaveLength(1);
    expect(parsed.nextPageToken).toBe('token-1');
  });
});
