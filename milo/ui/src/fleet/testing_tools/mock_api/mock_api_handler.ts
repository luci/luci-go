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

/**
 * Unified Fleet Console Mock API Layer (FleetConsoleMockAPI)
 *
 * Single source of truth for mocking Fleet Console pRPC endpoints across:
 * 1. Live C4A Starter static prototypes (go/fcon-prototypes)
 * 2. Jest & Vitest frontend unit tests
 * 3. Cypress end-to-end smoke tests
 *
 * Mandatory Maintenance Rule (AGENTS.md):
 * Whenever an agent adds or modifies a pRPC method in service.proto,
 * the corresponding mock fixture MUST be added or updated in this class.
 */

export interface FleetConsoleMockFixtures {
  CountDevices: unknown;
  CountBrowserDevices: unknown;
  ListDevices: unknown;
  ListAndroidDevices: unknown;
  ListBrowserDevices: unknown;
  GetBrowserDeviceDimensions: unknown;
  ExportBrowserDevicesToCSV: unknown;
  ExportDevicesToCSV: unknown;
  CountRepairMetrics: unknown;
  ListRepairMetrics: unknown;
  ListResourceRequests: unknown;
  ListWorkspaces: unknown;
  GetWorkspace: unknown;
  [method: string]: unknown;
}

const DEFAULT_AUTH_STATE = {
  identity: 'user:zhangtiff@google.com',
  email: 'zhangtiff@google.com',
  picture: '',
  accessToken: 'mock-access-token',
  idToken: 'mock-id-token',
  accessTokenExpiry: 9999999999,
  idTokenExpiry: 9999999999,
};

const DEFAULT_FIXTURES: FleetConsoleMockFixtures = {
  CountDevices: {
    total: 12536,
    androidTotal: 8420,
    chromeosTotal: 4116,
  },
  CountBrowserDevices: {
    total: 10,
    swarmingState: {
      total: 10,
      alive: 8,
      dead: 2,
      quarantined: 0,
      maintenance: 0,
    },
  },
  ListDevices: {
    devices: [
      {
        id: 'chromeos-device-01',
        dutId: 'dut-312323',
        deviceSpec: {
          labels: {
            model: { values: ['brya'] },
            board: { values: ['brya'] },
          },
        },
      },
      {
        id: 'chromeos-device-02',
        dutId: 'dut-312324',
        deviceSpec: {
          labels: {
            model: { values: ['corsola'] },
            board: { values: ['corsola'] },
          },
        },
      },
    ],
    nextPageToken: '',
  },
  ListAndroidDevices: {
    devices: [
      {
        id: 'android-device-01',
        deviceSpec: {
          ufsLabels: { hostname: { values: ['android-host-1'] } },
        },
      },
    ],
    nextPageToken: '',
  },
  ListBrowserDevices: {
    devices: [
      {
        id: '1',
        deviceId: 'browser-1',
        ufsLabels: { hostname: { values: ['browser-host-1'] } },
        swarmingLabels: { os: { values: ['Linux'] } },
      },
      {
        id: '2',
        deviceId: 'browser-2',
        ufsLabels: { hostname: { values: ['browser-host-2'] } },
        swarmingLabels: { os: { values: ['Windows'] } },
      },
    ],
    nextPageToken: '',
  },
  GetBrowserDeviceDimensions: {
    baseDimensions: {
      os: { values: ['Linux', 'Windows'] },
    },
    swarmingLabels: {},
    ufsLabels: {},
  },
  ExportBrowserDevicesToCSV: {
    csvData: 'id,device_id\n1,browser-1\n2,browser-2\n',
  },
  ExportDevicesToCSV: {
    csvData: 'id,dut_id\n1,dut-1\n2,dut-2\n',
  },
  CountRepairMetrics: {
    total: 3,
  },
  ListRepairMetrics: {
    repairMetrics: [
      {
        priority: 1,
        labName: 'sjc-mdpt9-wear',
        hostGroup: 'group1',
        runTarget: 'target1',
        minimumRepairs: 1,
        devicesOffline: 1,
        totalDevices: 2,
        peakUsage: 1,
      },
    ],
    nextPageToken: '',
  },
  ListResourceRequests: {
    requests: [
      {
        id: 'req-001',
        name: 'req-001',
        resourceName: 'pixel-8-pro',
        status: 1,
        amountRequested: 50,
        amountDelivered: 45,
      },
    ],
    nextPageToken: '',
  },
  ListWorkspaces: {
    workspaces: [
      {
        id: 'chromeos-core',
        name: 'chromeos-core',
        totalDevices: 4116,
      },
      {
        id: 'android-mobile',
        name: 'android-mobile',
        totalDevices: 8420,
      },
    ],
    nextPageToken: '',
  },
  GetWorkspace: {
    id: 'chromeos-core',
    name: 'chromeos-core',
    totalDevices: 4116,
  },
};

function deepClone<T>(obj: T): T {
  if (typeof structuredClone === 'function') {
    return structuredClone(obj);
  }
  return JSON.parse(JSON.stringify(obj));
}

export class FleetConsoleMockAPI {
  private static fixtures: Record<string, unknown> =
    deepClone(DEFAULT_FIXTURES);
  private static interceptorEnabled = false;

  /**
   * Resets all fixtures to default values.
   */
  static resetFixtures(): void {
    this.fixtures = deepClone(DEFAULT_FIXTURES);
  }

  /**
   * Overrides or adds a fixture for a specific pRPC method.
   */
  static setFixture(method: string, data: unknown): void {
    this.fixtures[method] = data;
  }

  /**
   * Gets the currently configured fixture for a method.
   */
  static getFixture(method: string): unknown {
    return this.fixtures[method];
  }

  /**
   * Enables the universal browser globalThis.fetch interceptor for pRPC calls.
   * Can be safely called in settings.js, Cypress beforeEach, or Jest setups.
   */
  static enableBrowserInterceptor(): void {
    if (this.interceptorEnabled) return;
    this.interceptorEnabled = true;

    const nativeFetch = globalThis.fetch.bind(globalThis);

    globalThis.fetch = async function (resource: unknown, init?: unknown) {
      const url =
        typeof resource === 'string'
          ? resource
          : (resource as { url?: string })?.url || '';

      // 1. Intercept openid auth state
      if (
        url.includes('/auth/openid/state') ||
        url.endsWith('/auth/openid/state')
      ) {
        return new Response(JSON.stringify(DEFAULT_AUTH_STATE), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        });
      }

      // 2. Intercept Fleet Console pRPC calls
      if (
        url.includes('/prpc/fleetconsole.FleetConsole/') ||
        url.includes('/prpc/fleet.FleetConsole/')
      ) {
        const method = url.split('/').pop() || '';
        const data = FleetConsoleMockAPI.getFixture(method) || {};

        // LUCI pRPC clients expect the )]}' prefix on JSON responses
        return new Response(")]}'\n" + JSON.stringify(data), {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
            'X-Prpc-Grpc-Code': '0',
          },
        });
      }

      return nativeFetch(resource as RequestInfo | URL, init as RequestInit);
    };
  }
}
