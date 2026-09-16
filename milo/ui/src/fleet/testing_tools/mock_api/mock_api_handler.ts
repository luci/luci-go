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
  CountAndroidDevices: unknown;
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
  BatchDeleteDevices?: unknown;
  GetDevice: unknown;
  UpdateDevice: unknown;
  ListAdminTasks: unknown;
  ListCustomerSlos: unknown;
  GetAdminTask: unknown;
  CreateAdminTask: unknown;
  BatchCreateAdminTasks: unknown;
  GetProductCatalogue: unknown;
  CreateOrder: unknown;
  Ping: unknown;
  PingBigQuery: unknown;
  PingDeviceManager: unknown;
  PingUfs: unknown;
  PingSwarming: unknown;
  GetDeviceDimensions: unknown;
  RepopulateCache: unknown;
  RepopulateBrowserCache: unknown;
  RepopulateAndroidCache: unknown;
  RepopulateAndroidDeviceUtilization: unknown;
  PingDB: unknown;
  CleanExit: unknown;
  ExportAndroidDevicesToCSV: unknown;
  ScheduleAutorepair: unknown;
  ScheduleReserve: unknown;
  ScheduleDeploy: unknown;
  UpdateAndroidMetrics: unknown;
  CheckPermission: unknown;
  CountResourceRequests: unknown;
  GetResourceRequestsMultiselectFilterValues: unknown;
  UpdateAndroidDevices: unknown;
  GetRepairMetricsDimensions: unknown;
  CleanupAndroidDevices: unknown;
  ScheduleBuild: unknown;
  ListProductCatalogEntries: unknown;
  GetProductCatalogFilterValues: unknown;
  ListGceProductCatalogEntries: unknown;
  GetGceProductCatalogFilterValues: unknown;
  GetSmartRepair: unknown;
  GetDeviceACLs: unknown;
  UpdateChromeOSDevice: unknown;
  ListRepairQueue: unknown;
  ClaimRepairTask: unknown;
  UnclaimRepairTask: unknown;
  ListPriorityRules: unknown;
  CreatePriorityRule: unknown;
  UpdatePriorityRule: unknown;
  DeletePriorityRule: unknown;
  GetDefaultQuota: unknown;
  SetDefaultQuota: unknown;
  ListIrmIncidents: unknown;
  ListSupportRiskIncidents: unknown;
  ListModelQuotaOverrides: unknown;
  SetModelQuotaOverride: unknown;
  DeleteModelQuotaOverride: unknown;
  [method: string]: unknown;
}

const DEFAULT_AUTH_STATE = {
  identity: 'user:user@example.com',
  email: 'user@example.com',
  picture: '',
  accessToken: 'mock-access-token',
  idToken: 'mock-id-token',
  accessTokenExpiry: 9999999999,
  idTokenExpiry: 9999999999,
};

import sampleAndroidDevices from './data/android_devices.json';
import sampleBrowserDevices from './data/browser_devices.json';
import sampleChromeosDevices from './data/chromeos_devices.json';
import sampleProductCatalog from './data/product_catalog.json';
import sampleRepairMetrics from './data/repair_metrics.json';
import sampleResourceRequests from './data/resource_requests.json';

const androidReadyCount = sampleAndroidDevices.filter(
  (d) => (d as { state?: string }).state === 'IDLE',
).length;
const androidBusyCount = sampleAndroidDevices.filter(
  (d) => (d as { state?: string }).state === 'BUSY',
).length;
const androidInTransitionCount = sampleAndroidDevices.filter(
  (d) => (d as { state?: string }).state === 'INIT',
).length;
const androidInAutoRecoveryCount = sampleAndroidDevices.filter(
  (d) => (d as { state?: string }).state === 'LAMEDUCK',
).length;
const androidNeedManualRepairCount = sampleAndroidDevices.filter((d) =>
  ['MISSING', 'FAILED', 'DYING'].includes(
    (d as { state?: string }).state || '',
  ),
).length;

const getBrowserStateCount = (stateName: string) =>
  sampleBrowserDevices.filter(
    (d) =>
      (
        d as {
          swarmingLabels?: { state?: { values?: readonly string[] } };
        }
      ).swarmingLabels?.state?.values?.[0] === stateName,
  ).length;

const sampleChromeosModels = Array.from(
  new Set(
    sampleChromeosDevices
      .map(
        (d) =>
          (
            d as {
              deviceSpec?: {
                labels?: { 'label-model'?: { values?: readonly string[] } };
              };
            }
          ).deviceSpec?.labels?.['label-model']?.values?.[0],
      )
      .filter((v): v is string => Boolean(v)),
  ),
);

const sampleChromeosBoards = Array.from(
  new Set(
    sampleChromeosDevices
      .map(
        (d) =>
          (
            d as {
              deviceSpec?: {
                labels?: { 'label-board'?: { values?: readonly string[] } };
              };
            }
          ).deviceSpec?.labels?.['label-board']?.values?.[0],
      )
      .filter((v): v is string => Boolean(v)),
  ),
);

const DEFAULT_FIXTURES: FleetConsoleMockFixtures = {
  CountDevices: {
    total: sampleChromeosDevices.length + sampleAndroidDevices.length,
    chromeosCount: {
      total: sampleChromeosDevices.length,
    },
    androidCount: {
      totalDevices: sampleAndroidDevices.length,
    },
    androidTotal: sampleAndroidDevices.length,
    chromeosTotal: sampleChromeosDevices.length,
  },
  CountAndroidDevices: {
    totalDevices: sampleAndroidDevices.length,
    totalHosts: Math.ceil(sampleAndroidDevices.length / 4),
    labMissingHosts: 0,
    labRunningHosts: Math.ceil(sampleAndroidDevices.length / 4),
    healthCategoryInService: {
      total: androidReadyCount + androidBusyCount,
      statusCounts: {
        'in service (ready)': androidReadyCount,
        'in service (busy)': androidBusyCount,
      },
    },
    healthCategoryInTransition: {
      total: androidInTransitionCount,
      statusCounts: {
        'in transition (installing)': androidInTransitionCount,
      },
    },
    healthCategoryInAutoRecovery: {
      total: androidInAutoRecoveryCount,
      statusCounts: {
        'in auto recovery': androidInAutoRecoveryCount,
      },
    },
    healthCategoryNeedManualRepair: {
      total: androidNeedManualRepairCount,
      statusCounts: {
        'need manual repair': androidNeedManualRepairCount,
      },
    },
    healthCategoryUnspecified: {
      total: 0,
      statusCounts: {},
    },
  },
  CountBrowserDevices: {
    total: sampleBrowserDevices.length,
    swarmingState: {
      total: sampleBrowserDevices.length,
      alive: getBrowserStateCount('alive'),
      dead: getBrowserStateCount('dead'),
      quarantined: getBrowserStateCount('quarantined'),
      maintenance: getBrowserStateCount('maintenance'),
    },
  },
  ListDevices: {
    devices: sampleChromeosDevices,
    nextPageToken: '',
  },
  ListAndroidDevices: {
    devices: sampleAndroidDevices,
    nextPageToken: '',
  },
  ListBrowserDevices: {
    devices: sampleBrowserDevices,
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
    total: sampleRepairMetrics.length,
  },
  ListRepairMetrics: {
    repairMetrics: sampleRepairMetrics,
    nextPageToken: '',
  },
  CountResourceRequests: {
    total: sampleResourceRequests.length,
  },
  ListResourceRequests: {
    resourceRequests: sampleResourceRequests,
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
  BatchDeleteDevices: {},
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
  ListCustomerSlos: {
    customerSlos: [],
    nextPageToken: '',
    totalSize: 0,
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
  Ping: {},
  PingBigQuery: {},
  PingDeviceManager: {},
  PingUfs: {},
  PingSwarming: {},
  GetDeviceDimensions: {
    baseDimensions: {
      model: { values: sampleChromeosModels },
      board: { values: sampleChromeosBoards },
    },
    swarmingLabels: {},
    ufsLabels: {},
  },
  RepopulateCache: {},
  RepopulateBrowserCache: {},
  RepopulateAndroidCache: {},
  RepopulateAndroidDeviceUtilization: {},
  PingDB: {},
  CleanExit: {},
  ExportAndroidDevicesToCSV: {
    csvData: 'id,hostname\n1,android-1\n',
  },
  ScheduleAutorepair: {},
  ScheduleReserve: {},
  ScheduleDeploy: {},
  UpdateAndroidMetrics: {},
  CheckPermission: {
    hasPermission: true,
  },
  GetResourceRequestsMultiselectFilterValues: {
    filterValues: {},
  },
  UpdateAndroidDevices: {},
  GetRepairMetricsDimensions: {
    dimensions: {},
  },
  CleanupAndroidDevices: {},
  ScheduleBuild: {},
  ListProductCatalogEntries: {
    entries: sampleProductCatalog,
    nextPageToken: '',
  },
  GetProductCatalogFilterValues: {
    filterValues: {},
  },
  ListGceProductCatalogEntries: {
    entries: [],
    nextPageToken: '',
  },
  GetGceProductCatalogFilterValues: {
    filterValues: {},
  },
  GetSmartRepair: {
    url: 'https://example.com/smart-repair',
  },
  GetDeviceACLs: {
    acls: [],
  },
  UpdateChromeOSDevice: {},
  ListRepairQueue: {
    tasks: [],
    nextPageToken: '',
  },
  ClaimRepairTask: {
    task: {
      id: 'task-claimed-001',
    },
  },
  UnclaimRepairTask: {
    task: {
      id: 'task-unclaimed-001',
    },
  },
  ListPriorityRules: {
    priorityRules: [],
    nextPageToken: '',
  },
  CreatePriorityRule: {
    priorityRule: {
      id: 'rule-001',
    },
  },
  UpdatePriorityRule: {
    priorityRule: {
      id: 'rule-001',
    },
  },
  DeletePriorityRule: {},
  GetDefaultQuota: {
    defaultQuota: 49,
  },
  SetDefaultQuota: {
    defaultQuota: 49,
  },
  ListIrmIncidents: {
    irmIncidents: [],
  },
  ListSupportRiskIncidents: {
    incidents: [
      {
        id: '1',
        model: 'brya',
        buganizerId: '1001',
        title: 'Brya trackpad failure batch',
      },
      {
        id: '2',
        model: 'brask',
        buganizerId: '1002',
        title: 'Brask USB controller glitch',
      },
      {
        id: '3',
        model: 'asurada',
        buganizerId: '1003',
        title: 'Asurada battery swelling campaign',
      },
      {
        id: '4',
        model: 'atlas',
        buganizerId: '1004',
        title: 'Atlas firmware crash loop',
      },
      {
        id: '5',
        model: 'volteer',
        buganizerId: '1005',
        title: 'Volteer Type-C power delivery failure',
      },
      {
        id: '6',
        model: 'dedede',
        buganizerId: '1006',
        title: 'Dedede eMMC storage read degradation',
      },
      {
        id: '7',
        model: 'zork',
        buganizerId: '1007',
        title: 'Zork thermal throttle BIOS bug',
      },
      {
        id: '8',
        model: 'hatch',
        buganizerId: '1008',
        title: 'Hatch audio codec kernel panic',
      },
      {
        id: '9',
        model: 'puff',
        buganizerId: '1009',
        title: 'Puff fan bearing degradation',
      },
      {
        id: '10',
        model: 'octopus',
        buganizerId: '1010',
        title: 'Octopus display flicker on wake',
      },
      {
        id: '11',
        model: 'nissa',
        buganizerId: '1011',
        title: 'Nissa Wi-Fi module disconnection',
      },
      {
        id: '12',
        model: 'guybrush',
        buganizerId: '1012',
        title: 'Guybrush sleep state power drain',
      },
      {
        id: '13',
        model: 'skyrim',
        buganizerId: '1013',
        title: 'Skyrim camera sensor detection loss',
      },
      {
        id: '14',
        model: 'corsola',
        buganizerId: '1014',
        title: 'Corsola keyboard matrix ghosting',
      },
      {
        id: '15',
        model: 'geralt',
        buganizerId: '1015',
        title: 'Geralt touch digitizer deadzone',
      },
    ],
  },
  ListModelQuotaOverrides: {
    overrides: [
      {
        id: '1',
        model: 'brya',
        overriddenExpectedQuantity: 15,
      },
      {
        id: '2',
        model: 'volteer',
        overriddenExpectedQuantity: 20,
      },
    ],
  },
  SetModelQuotaOverride: {
    override: {
      id: '1',
      model: 'brya',
      overriddenExpectedQuantity: 15,
    },
  },
  DeleteModelQuotaOverride: {},
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
        let reqPayload: Record<string, unknown> = {};
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
            // Ignore non-JSON body parse errors
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

        let responseData: Record<string, unknown> =
          typeof data === 'object' && data !== null
            ? (deepClone(data) as Record<string, unknown>)
            : {};

        // In-memory AIP-160 filter evaluation & pagination simulation (for static object fixtures)
        if (typeof fixture !== 'function') {
          const filterStr =
            typeof reqPayload.filter === 'string' ? reqPayload.filter : '';
          const listArrayKey = Object.keys(responseData).find((k) =>
            Array.isArray(responseData[k]),
          );

          if (listArrayKey && Array.isArray(responseData[listArrayKey])) {
            let items = responseData[listArrayKey] as Array<
              Record<string, unknown>
            >;

            if (filterStr) {
              // Simple AIP-160 substring match: matches any exact string literal in filter
              const stringMatches =
                filterStr.match(/"([^"]+)"/) || filterStr.match(/=\s*([^\s]+)/);
              if (stringMatches && stringMatches[1]) {
                const targetVal = stringMatches[1].toLowerCase();
                items = items.filter((item) =>
                  JSON.stringify(item).toLowerCase().includes(targetVal),
                );
              }
            }

            const pageSize =
              typeof reqPayload.pageSize === 'number' && reqPayload.pageSize > 0
                ? reqPayload.pageSize
                : items.length;
            const startIndex =
              typeof reqPayload.pageToken === 'string' &&
              reqPayload.pageToken.startsWith('token-')
                ? parseInt(reqPayload.pageToken.replace('token-', ''), 10) || 0
                : 0;

            const pagedItems = items.slice(startIndex, startIndex + pageSize);
            const nextIndex = startIndex + pageSize;
            const nextPageToken =
              nextIndex < items.length ? `token-${nextIndex}` : '';

            responseData = {
              ...responseData,
              [listArrayKey]: pagedItems,
              nextPageToken,
              totalSize: items.length,
            };
          }
        }

        // LUCI pRPC clients expect the )]}' prefix on JSON responses
        return new Response(")]}'\n" + JSON.stringify(responseData), {
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
