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
  GetDevice: unknown;
  UpdateDevice: unknown;
  ListAdminTasks: unknown;
  GetAdminTask: unknown;
  CreateAdminTask: unknown;
  BatchCreateAdminTasks: unknown;
  GetProductCatalogue: unknown;
  CreateOrder: unknown;
  [method: string]: unknown;
}

const DEFAULT_AUTH_STATE = {
  identity: 'user:user@google.com',
  email: 'user@google.com',
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
  GetDevice: {
    device: {
      id: 'chromeos-device-01',
      dutId: 'dut-312323',
    },
  },
  UpdateDevice: {
    device: {
      id: 'chromeos-device-01',
      dutId: 'dut-312323',
    },
  },
  ListAdminTasks: {
    tasks: [
      {
        id: 'task-101',
        taskType: 'REPAIR',
        targetDeviceId: 'chromeos-device-01',
        status: 'COMPLETED',
        createdAt: '2026-08-10T12:00:00Z',
      },
    ],
    nextPageToken: '',
  },
  GetAdminTask: {
    task: {
      id: 'task-101',
      taskType: 'REPAIR',
      targetDeviceId: 'chromeos-device-01',
      status: 'COMPLETED',
    },
  },
  CreateAdminTask: {
    task: {
      id: 'task-102',
      status: 'QUEUED',
    },
  },
  BatchCreateAdminTasks: {
    tasks: [
      {
        id: 'task-103',
        status: 'QUEUED',
      },
    ],
  },
  GetProductCatalogue: {
    products: [
      {
        id: 'prod-001',
        name: 'Chromebook Enterprise',
        category: 'ChromeOS',
      },
    ],
  },
  CreateOrder: {
    orderId: 'order-999',
    status: 'SUBMITTED',
  },
};

function deepClone<T>(obj: T): T {
  if (typeof structuredClone === 'function') {
    return structuredClone(obj);
  }
  return JSON.parse(JSON.stringify(obj));
}

export interface PrototypeOptions {
  /** If true, persists fixture mutations to localStorage */
  persistToLocalStorage?: boolean;
  /** Key for localStorage state storage */
  localStorageKey?: string;
  /** Simulated network latency in milliseconds */
  latencyMs?: number;
  /** Custom initial auth state for prototype user context */
  authState?: Record<string, unknown>;
  /** Debug logging to console */
  debug?: boolean;
}

export interface MockCallRecord {
  url: string;
  payload: unknown;
  headers?: unknown;
}

export class FleetConsoleMockAPI {
  private static fixtures: Record<string, unknown> =
    deepClone(DEFAULT_FIXTURES);
  private static currentAuthState: Record<string, unknown> =
    deepClone(DEFAULT_AUTH_STATE);
  private static callHistory: Record<string, MockCallRecord[]> = {};
  private static interceptorEnabled = false;
  private static latencyMs = 0;
  private static prototypeMode = false;
  private static persistStorage = false;
  private static storageKey = 'fcon_prototype_mock_fixtures';
  private static nativeFetch: typeof globalThis.fetch | null = null;

  /**
   * Initializes FleetConsoleMockAPI for standalone static UI prototyping.
   */
  static initPrototypeMode(options: PrototypeOptions = {}): void {
    this.prototypeMode = true;
    this.persistStorage = options.persistToLocalStorage ?? true;
    if (options.localStorageKey) {
      this.storageKey = options.localStorageKey;
    }
    if (options.latencyMs !== undefined) {
      this.latencyMs = options.latencyMs;
    }
    if (options.authState) {
      this.setAuthState(options.authState);
    }

    if (
      this.persistStorage &&
      typeof window !== 'undefined' &&
      window.localStorage
    ) {
      try {
        const saved = window.localStorage.getItem(this.storageKey);
        if (saved) {
          this.fixtures = {
            ...deepClone(DEFAULT_FIXTURES),
            ...JSON.parse(saved),
          };
        }
      } catch (e) {
        if (options.debug) {
          /* eslint-disable no-console */
          console.warn('Failed to restore mock state from localStorage', e);
          /* eslint-enable no-console */
        }
      }
    }

    this.enableBrowserInterceptor();

    if (typeof window !== 'undefined') {
      (
        window as unknown as { FleetConsoleMockAPI: typeof FleetConsoleMockAPI }
      ).FleetConsoleMockAPI = FleetConsoleMockAPI;
    }
  }

  /**
   * Returns true if FleetConsoleMockAPI was initialized in prototype mode.
   */
  static isPrototypeMode(): boolean {
    return this.prototypeMode;
  }

  /**
   * Resets all fixtures to default values and clears call history.
   */
  static resetFixtures(): void {
    this.fixtures = deepClone(DEFAULT_FIXTURES);
    this.currentAuthState = deepClone(DEFAULT_AUTH_STATE);
    this.callHistory = {};
    if (
      this.persistStorage &&
      typeof window !== 'undefined' &&
      window.localStorage
    ) {
      try {
        window.localStorage.removeItem(this.storageKey);
      } catch {
        // Ignore localStorage clear failures
      }
    }
  }

  /**
   * Clears call history records.
   */
  static clearCalls(): void {
    this.callHistory = {};
  }

  /**
   * Returns all recorded calls for a method.
   */
  static getCalls(method: string): MockCallRecord[] {
    return this.callHistory[method] || [];
  }

  /**
   * Records an intercepted pRPC call.
   */
  private static recordCall(method: string, call: MockCallRecord): void {
    if (!this.callHistory[method]) {
      this.callHistory[method] = [];
    }
    this.callHistory[method].push(call);
  }

  /**
   * Sets mock auth state for OpenID state endpoint interception.
   */
  static setAuthState(stateOverride: Record<string, unknown>): void {
    this.currentAuthState = {
      ...this.currentAuthState,
      ...stateOverride,
    };
  }

  /**
   * Resets auth state back to default logged-in identity.
   */
  static resetAuthState(): void {
    this.currentAuthState = deepClone(DEFAULT_AUTH_STATE);
  }

  /**
   * Sets an error fixture for a pRPC method.
   */
  static setErrorFixture(
    method: string,
    err: { message?: string; grpcCode?: number | string },
  ): void {
    this.setFixture(method, { __isError: true, ...err });
  }

  /**
   * Overrides or adds a fixture for a specific pRPC method.
   */
  static setFixture(method: string, data: unknown): void {
    this.fixtures[method] = data;
    if (
      this.persistStorage &&
      typeof window !== 'undefined' &&
      window.localStorage
    ) {
      try {
        window.localStorage.setItem(
          this.storageKey,
          JSON.stringify(this.fixtures),
        );
      } catch {
        // Ignore localStorage quota / availability failures in test environments
      }
    }
  }

  /**
   * Disables browser interceptor and restores native fetch.
   */
  static disableBrowserInterceptor(): void {
    if (this.interceptorEnabled) {
      this.interceptorEnabled = false;
      if (this.nativeFetch) {
        globalThis.fetch = this.nativeFetch;
        this.nativeFetch = null;
      }
    }
    this.prototypeMode = false;
    this.latencyMs = 0;
    this.persistStorage = false;
    this.storageKey = 'fcon_prototype_mock_fixtures';
  }

  /**
   * Gets the currently configured fixture for a method.
   */
  static getFixture(method: string): unknown {
    const fixture = this.fixtures[method];
    if (
      fixture !== null &&
      typeof fixture === 'object' &&
      typeof (fixture as Promise<unknown>).then !== 'function'
    ) {
      return deepClone(fixture);
    }
    return fixture;
  }

  /**
   * Enables the universal browser globalThis.fetch interceptor for pRPC calls.
   * Can be safely called in settings.js, Cypress beforeEach, or Jest setups.
   */
  static enableBrowserInterceptor(): void {
    if (this.interceptorEnabled) return;

    // Safety guardrail: Do not intercept requests in production environments unless explicitly in prototype mode.
    if (process.env.NODE_ENV === 'production' && !this.prototypeMode) {
      /* eslint-disable no-console */
      console.warn(
        '[FleetConsoleMockAPI] Interceptor disabled in production build.',
      );
      /* eslint-enable no-console */
      return;
    }
    this.interceptorEnabled = true;
    this.nativeFetch = globalThis.fetch;

    const originalFetch = this.nativeFetch.bind(globalThis);

    globalThis.fetch = async function (resource: unknown, options?: unknown) {
      const url =
        typeof resource === 'string'
          ? resource
          : (resource as { url?: string })?.url || '';

      if (FleetConsoleMockAPI.latencyMs > 0) {
        await new Promise((resolve) =>
          setTimeout(resolve, FleetConsoleMockAPI.latencyMs),
        );
      }

      // 1. Intercept openid auth state
      if (
        url.includes('/auth/openid/state') ||
        url.endsWith('/auth/openid/state')
      ) {
        return new Response(
          JSON.stringify(FleetConsoleMockAPI.currentAuthState),
          {
            status: 200,
            headers: { 'content-type': 'application/json' },
          },
        );
      }

      // 2. Intercept pRPC calls
      if (url.includes('/prpc/')) {
        const method = (url.split('/').pop() || '').split('?')[0];
        const opts = options as RequestInit | undefined;
        let reqPayload: unknown = {};
        if (opts?.body) {
          try {
            const bodyStr =
              typeof opts.body === 'string'
                ? opts.body
                : opts.body instanceof URLSearchParams
                  ? opts.body.toString()
                  : '';
            if (bodyStr) {
              reqPayload = JSON.parse(bodyStr);
            }
          } catch {
            reqPayload = opts.body;
          }
        }

        FleetConsoleMockAPI.recordCall(method, {
          url,
          payload: reqPayload,
          headers: opts?.headers,
        });

        const fixture = FleetConsoleMockAPI.getFixture(method);

        let data: unknown = fixture;
        if (typeof fixture === 'function') {
          try {
            data = fixture(reqPayload, options);
          } catch (err) {
            const errorMsg = err instanceof Error ? err.message : String(err);
            return new Response(
              ")]}'\n" + JSON.stringify({ message: errorMsg }),
              {
                status: 500,
                headers: {
                  'Content-Type': 'application/json',
                  'X-Prpc-Grpc-Code': '13',
                },
              },
            );
          }
        }

        if (
          typeof data === 'object' &&
          data !== null &&
          typeof (data as Promise<unknown>).then === 'function'
        ) {
          try {
            data = await data;
          } catch (err) {
            const errorMsg = err instanceof Error ? err.message : String(err);
            return new Response(
              ")]}'\n" + JSON.stringify({ message: errorMsg }),
              {
                status: 500,
                headers: {
                  'Content-Type': 'application/json',
                  'X-Prpc-Grpc-Code': '13',
                },
              },
            );
          }
        }

        if (data instanceof Error) {
          return new Response(
            ")]}'\n" + JSON.stringify({ message: data.message }),
            {
              status: 500,
              headers: {
                'Content-Type': 'application/json',
                'X-Prpc-Grpc-Code': '13',
              },
            },
          );
        }

        if (
          data === null ||
          (typeof data === 'object' &&
            data !== null &&
            (data as Record<string, unknown>).__isError)
        ) {
          const errObj = (data as Record<string, unknown>) || {};
          const grpcCode = String(errObj.grpcCode ?? '13');
          const msg = String(errObj.message ?? 'pRPC Server Error');
          return new Response(")]}'\n" + JSON.stringify({ message: msg }), {
            status: 500,
            headers: {
              'Content-Type': 'application/json',
              'X-Prpc-Grpc-Code': grpcCode,
            },
          });
        }

        const fixtureData = data || {};

        // LUCI pRPC clients expect the )]}' prefix on JSON responses
        return new Response(")]}'\n" + JSON.stringify(fixtureData), {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
            'X-Prpc-Grpc-Code': '0',
          },
        });
      }

      return originalFetch(
        resource as RequestInfo | URL,
        options as RequestInit,
      );
    };
  }
}
