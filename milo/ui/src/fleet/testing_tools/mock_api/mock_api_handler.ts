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
 * 1. Live C4A Starter static prototypes
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
  ListAndroidRepairQueue: unknown;
  ClaimAndroidRepairTask: unknown;
  UnclaimAndroidRepairTask: unknown;
  ListAndroidPriorityRules: unknown;
  CreateAndroidPriorityRule: unknown;
  UpdateAndroidPriorityRule: unknown;
  DeleteAndroidPriorityRule: unknown;
  GetDefaultQuota: unknown;
  SetDefaultQuota: unknown;
  ListIrmIncidents: unknown;
  ListSupportRiskIncidents: unknown;
  ListModelQuotaOverrides: unknown;
  SetModelQuotaOverride: unknown;
  DeleteModelQuotaOverride: unknown;
  GetWorkforceActivity: unknown;
  GetFleetAvailabilityTrends: unknown;
  ListTaskHistory: unknown;
  ListHealthSlices: unknown;
  CreateHealthSlice: unknown;
  UpdateHealthSlice: unknown;
  DeleteHealthSlice: unknown;
  ReorderHealthSlices: unknown;
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

import * as aip160Ast from '../../utils/aip160/ast/ast';
import { parseFilter } from '../../utils/aip160/parser/parser';

import sampleAndroidDevices from './data/android_devices.json';
import sampleBrowserDevices from './data/browser_devices.json';
import sampleChromeosDevices from './data/chromeos_devices.json';
import sampleProductCatalog from './data/product_catalog.json';
import sampleRepairMetrics from './data/repair_metrics.json';
import sampleResourceRequests from './data/resource_requests.json';
import {
  applyHealthSliceMutation,
  buildDynamicGetBuild,
  buildDynamicListBots,
  buildDynamicMachineLse,
  buildMockAvailabilityTrends,
  HEALTH_AND_TASK_DEFAULT_FIXTURES,
  synthesizeChromeosDevice,
} from './mock_health_and_tasks';

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

const sampleChromeosLabels: Record<string, { values: string[] }> = {};
for (const d of sampleChromeosDevices) {
  const labels = (
    d as unknown as {
      deviceSpec?: {
        labels?: Record<string, { values?: readonly string[] } | undefined>;
      };
    }
  ).deviceSpec?.labels;
  if (!labels) continue;
  for (const [k, v] of Object.entries(labels)) {
    sampleChromeosLabels[k] ??= { values: [] };
    for (const val of v?.values || []) {
      if (val && !sampleChromeosLabels[k].values.includes(val)) {
        sampleChromeosLabels[k].values.push(val);
      }
    }
  }
}

function computeChromeosCounts(devices: Array<Record<string, unknown>>) {
  const byDutState = (st: string) =>
    devices.filter(
      (d) =>
        (
          d.deviceSpec as {
            labels?: Record<string, { values?: readonly string[] }>;
          }
        )?.labels?.dut_state?.values?.[0] === st,
    ).length;
  return {
    total: devices.length,
    deviceState: {
      ready: byDutState('ready'),
      needRepair: byDutState('needs_repair'),
      needManualRepair: byDutState('needs_manual_repair'),
      repairFailed: byDutState('repair_failed'),
    },
    taskState: {
      busy: devices.filter((d) => d.state === 'DEVICE_STATE_LEASED').length,
      idle: devices.filter((d) => d.state === 'DEVICE_STATE_AVAILABLE').length,
    },
  };
}

const defaultCrosCounts = computeChromeosCounts(
  sampleChromeosDevices as Array<Record<string, unknown>>,
);

const DEFAULT_FIXTURES: FleetConsoleMockFixtures = {
  CountDevices: {
    ...defaultCrosCounts,
    total: sampleChromeosDevices.length + sampleAndroidDevices.length,
    chromeosCount: defaultCrosCounts,
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
    records: [
      {
        dimensions: {
          'ufs.resource_state': 'SERVING',
          bot_health_status: 'HEALTHY',
        },
        count: String(getBrowserStateCount('alive')),
      },
      {
        dimensions: {
          'ufs.resource_state': 'SERVING',
          bot_health_status: 'DEAD',
        },
        count: String(getBrowserStateCount('dead')),
      },
      {
        dimensions: {
          'ufs.resource_state': 'SERVING',
          bot_health_status: 'QUARANTINED',
        },
        count: String(getBrowserStateCount('quarantined')),
      },
      {
        dimensions: {
          'ufs.resource_state': 'SERVING',
          bot_health_status: 'MAINTENANCE',
        },
        count: String(getBrowserStateCount('maintenance')),
      },
    ],
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
    ufsLabels: {
      location_group_tag: { values: ['demo-location-1', 'mock-location-2'] },
    },
  },
  ExportBrowserDevicesToCSV: {
    csvData: 'id,device_id\n1,browser-1\n2,browser-2\n',
  },
  ExportDevicesToCSV: {
    csvData: 'id,dut_id\n1,dut-1\n2,dut-2\n',
  },
  CountRepairMetrics: {
    total: sampleRepairMetrics.length,
    totalDevices: sampleAndroidDevices.length,
    inServiceDevices: androidReadyCount + androidBusyCount,
    needManualRepairDevices: androidNeedManualRepairCount,
    inAutomatedMaintenanceDevices:
      androidInTransitionCount + androidInAutoRecoveryCount,
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
      id: { values: sampleChromeosDevices.map((d) => String(d.id)) },
      dut_id: { values: sampleChromeosDevices.map((d) => String(d.dutId)) },
      state: { values: ['DEVICE_STATE_AVAILABLE', 'DEVICE_STATE_LEASED'] },
      type: { values: ['DEVICE_TYPE_PHYSICAL'] },
      model: { values: sampleChromeosModels },
      board: { values: sampleChromeosBoards },
    },
    labels: {
      ...sampleChromeosLabels,
      location_group_tag: { values: ['demo-location-1', 'mock-location-2'] },
    },
    swarmingLabels: {
      ...sampleChromeosLabels,
      location_group_tag: { values: ['demo-location-1', 'mock-location-2'] },
    },
    ufsLabels: {
      ...sampleChromeosLabels,
      location_group_tag: { values: ['demo-location-1', 'mock-location-2'] },
      'ufs.location_group_tag': {
        values: ['demo-location-1', 'mock-location-2'],
      },
    },
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
  ListAndroidRepairQueue: {
    repairQueueItems: [],
    nextPageToken: '',
    totalSize: 0,
    inProgressCount: 0,
  },
  ClaimAndroidRepairTask: {
    repairQueueItem: {
      deviceId: 'android-device-001',
      fcMachineType: 'ANDROID_MACHINE_TYPE_DEVICE',
      hostGroups: ['group1'],
      runTarget: 'demo-target-1',
      state: 'MISSING',
      taskId: '1001',
      claimedBy: 'user@example.com',
      priorityScore: '10',
    },
  },
  UnclaimAndroidRepairTask: {
    repairQueueItem: {
      deviceId: 'android-device-001',
      fcMachineType: 'ANDROID_MACHINE_TYPE_DEVICE',
      hostGroups: ['group1'],
      runTarget: 'demo-target-1',
      state: 'MISSING',
      taskId: '1001',
      claimedBy: '',
      priorityScore: '10',
    },
  },
  ListAndroidPriorityRules: {
    priorityRules: [],
  },
  CreateAndroidPriorityRule: {
    priorityRule: {
      id: '1',
      expressionAip160: 'host_group = "group1"',
      weight: '10',
      workspace: 'ANDROID_WORKSPACE_ANDROID',
    },
  },
  UpdateAndroidPriorityRule: {
    priorityRule: {
      id: '1',
      expressionAip160: 'host_group = "group1"',
      weight: '10',
      workspace: 'ANDROID_WORKSPACE_ANDROID',
    },
  },
  DeleteAndroidPriorityRule: {},
  GetDefaultQuota: {
    defaultQuota: 49,
  },
  SetDefaultQuota: {
    defaultQuota: 49,
  },
  ListSupportRiskIncidents: {
    incidents: [
      {
        id: '1',
        model: 'sim-model-nine',
        buganizerId: '1001',
        title: 'Sim-model-nine trackpad failure batch',
      },
      {
        id: '2',
        model: 'demo-model-one',
        buganizerId: '1002',
        title: 'Demo-model-one USB controller glitch',
      },
      {
        id: '3',
        model: 'demo-model-seven',
        buganizerId: '1003',
        title: 'Demo-model-seven battery swelling campaign',
      },
      {
        id: '4',
        model: 'demo-model-ten',
        buganizerId: '1004',
        title: 'Demo-model-ten firmware crash loop',
      },
      {
        id: '5',
        model: 'mock-model-two',
        buganizerId: '1005',
        title: 'Mock-model-two Type-C power delivery failure',
      },
      {
        id: '6',
        model: 'sim-model-three',
        buganizerId: '1006',
        title: 'Sim-model-three eMMC storage read degradation',
      },
      {
        id: '7',
        model: 'sim-model-six',
        buganizerId: '1007',
        title: 'Sim-model-six thermal throttle BIOS bug',
      },
      {
        id: '8',
        model: 'mock-model-eight',
        buganizerId: '1008',
        title: 'Mock-model-eight audio codec kernel panic',
      },
      {
        id: '9',
        model: 'sim-model-labstation',
        buganizerId: '1009',
        title: 'Sim-model-labstation fan bearing degradation',
      },
      {
        id: '10',
        model: 'sim-model-satlab',
        buganizerId: '1010',
        title: 'Sim-model-satlab display flicker on wake',
      },
      {
        id: '11',
        model: 'mock-board-epsilon',
        buganizerId: '1011',
        title: 'Mock-board-epsilon Wi-Fi module disconnection',
      },
      {
        id: '12',
        model: 'demo-board-alpha',
        buganizerId: '1012',
        title: 'Demo-board-alpha sleep state power drain',
      },
      {
        id: '13',
        model: 'demo-board-delta',
        buganizerId: '1013',
        title: 'Demo-board-delta camera sensor detection loss',
      },
      {
        id: '14',
        model: 'sim-board-gamma',
        buganizerId: '1014',
        title: 'Sim-board-gamma keyboard matrix ghosting',
      },
      {
        id: '15',
        model: 'mock-board-theta',
        buganizerId: '1015',
        title: 'Mock-board-theta touch digitizer deadzone',
      },
    ],
  },
  ListModelQuotaOverrides: {
    overrides: [
      {
        id: '1',
        model: 'sim-model-nine',
        overriddenExpectedQuantity: 15,
      },
      {
        id: '2',
        model: 'demo-model-one',
        overriddenExpectedQuantity: 20,
      },
    ],
  },
  SetModelQuotaOverride: {
    override: {
      id: '1',
      model: 'sim-model-nine',
      overriddenExpectedQuantity: 15,
    },
  },
  DeleteModelQuotaOverride: {},
  GetWorkforceActivity: {
    activeRepairers: 4,
    totalPriorityPointsCleared: 2850,
    avgQueuePickupRank: 4.7,
    inProgressRepairs: 2,
    avgRepairTime: { seconds: '1680', nanos: 0 },
    technicians: [
      {
        id: 'andrew',
        name: 'Andrew Miller',
        email: 'andrew@google.com',
        claimedDuts: [
          {
            dutId: 'demo-cros-r2-rk1-h2',
            score: 250,
            duration: { seconds: '2700', nanos: 0 },
          },
        ],
        priorityScoreCleared: 680,
        avgPtsPerDut: 85,
        avgQueuePickupRank: 1.8,
        pickupRankSum: 14,
        pickupRankLabel: 'Top Priority',
        isRankSkewed: false,
        completedRepairs: 8,
        avgDuration: { seconds: '1920', nanos: 0 },
      },
      {
        id: 'beatrice',
        name: 'Beatrice Chen',
        email: 'beatrice@google.com',
        claimedDuts: [
          {
            dutId: 'demo-cros-r3-rk1-h3',
            score: 85,
            duration: { seconds: '1800', nanos: 0 },
          },
        ],
        priorityScoreCleared: 940,
        avgPtsPerDut: 85,
        avgQueuePickupRank: 1.2,
        pickupRankSum: 13,
        pickupRankLabel: 'Top Priority',
        isRankSkewed: true,
        completedRepairs: 11,
        avgDuration: { seconds: '1440', nanos: 0 },
      },
    ],
  },
  ...HEALTH_AND_TASK_DEFAULT_FIXTURES,
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
  private static overriddenMethods: Set<string> = new Set();
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
    this.overriddenMethods.clear();
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
    this.overriddenMethods.add(method);
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
   * Evaluates an AIP-160 filter expression against a mock item using the
   * canonical Fleet Console AIP-160 lexer and AST parser.
   */
  static matchesAip160Filter(
    item: Record<string, unknown>,
    filterStr: string,
  ): boolean {
    const trimmed = filterStr.trim();
    if (!trimmed) return true;
    const parsed = parseFilter(trimmed);
    if (parsed.isError || !parsed.ast.expression) {
      const fallback = trimmed.replace(/^"|"$/g, '').toLowerCase();
      return JSON.stringify(item).toLowerCase().includes(fallback);
    }

    const evalRestriction = (r: aip160Ast.Restriction): boolean => {
      const root = r.comparable.member.value.value;
      const fields = r.comparable.member.fields.map((f) => f.value);
      if (!r.comparator || !r.arg) {
        const needle = [root, ...fields].join('.').toLowerCase();
        return JSON.stringify(item).toLowerCase().includes(needle);
      }
      if (r.arg.kind === 'Expression') {
        return evalExpr(r.arg);
      }
      const op = r.comparator;
      const targetVal = [
        r.arg.member.value.value,
        ...r.arg.member.fields.map((f) => f.value),
      ]
        .join('.')
        .toLowerCase();
      const labelMap =
        (
          item.deviceSpec as {
            labels?: Record<string, { values?: readonly string[] }>;
          }
        )?.labels ||
        (item.swarmingLabels as
          | Record<string, { values?: readonly string[] }>
          | undefined) ||
        (
          item.omnilabSpec as {
            labels?: Record<string, { values?: readonly string[] }>;
          }
        )?.labels;

      let matched = false;
      if (root === 'labels' && fields.length > 0) {
        const vals = labelMap?.[fields.join('.')]?.values ?? [];
        matched =
          targetVal === '*'
            ? vals.length > 0
            : vals.some((v) => {
                const lv = String(v).toLowerCase();
                return op === ':' ? lv.includes(targetVal) : lv === targetVal;
              });
      } else if (root in item) {
        const fv = String(item[root] ?? '').toLowerCase();
        matched =
          targetVal === '*'
            ? Boolean(item[root])
            : op === ':'
              ? fv.includes(targetVal)
              : fv === targetVal || fv.endsWith(`_${targetVal}`);
      } else {
        matched = JSON.stringify(item).toLowerCase().includes(targetVal);
      }
      return op === '!=' ? !matched : matched;
    };

    const evalExpr = (expr: aip160Ast.Expression): boolean =>
      expr.sequences.every((seq) =>
        seq.factors.every((factor) =>
          factor.terms.some((term) => {
            const res =
              term.simple.kind === 'Expression'
                ? evalExpr(term.simple)
                : evalRestriction(term.simple);
            return term.negated ? !res : res;
          }),
        ),
      );

    return evalExpr(parsed.ast.expression);
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

        // Dynamic request-aware responses when default fixture has not been overridden
        if (
          typeof fixture !== 'function' &&
          !FleetConsoleMockAPI.overriddenMethods.has(method)
        ) {
          const sliceMutation = applyHealthSliceMutation(
            method,
            reqPayload,
            FleetConsoleMockAPI.fixtures,
          );
          if (sliceMutation !== null) {
            responseData = sliceMutation;
            if (
              FleetConsoleMockAPI.persistStorage &&
              typeof window !== 'undefined' &&
              window.localStorage
            ) {
              try {
                window.localStorage.setItem(
                  FleetConsoleMockAPI.storageKey,
                  JSON.stringify(FleetConsoleMockAPI.fixtures),
                );
              } catch {
                // Ignore localStorage quota / availability failures in test environments
              }
            }
          } else if (method === 'GetFleetAvailabilityTrends') {
            responseData = buildMockAvailabilityTrends(reqPayload);
          } else if (method === 'GetDevice') {
            const requestedId =
              typeof reqPayload.id === 'string'
                ? reqPayload.id
                : (reqPayload.id as { id?: string } | undefined)?.id || '';
            const allCros = sampleChromeosDevices as Array<
              Record<string, unknown>
            >;
            const device = synthesizeChromeosDevice(
              requestedId || 'demo-cros-r2-rk1-h2',
              allCros,
            );
            responseData = { device };
          } else if (method === 'GetMachineLSE') {
            const allCros = sampleChromeosDevices as Array<
              Record<string, unknown>
            >;
            responseData = buildDynamicMachineLse(
              reqPayload,
              responseData,
              allCros,
            );
          } else if (method === 'ListBots') {
            responseData = buildDynamicListBots(reqPayload);
          } else if (method === 'GetBuild') {
            responseData = buildDynamicGetBuild(reqPayload, responseData);
          } else if (
            method === 'CountBrowserDevices' &&
            Array.isArray(reqPayload.groupBy) &&
            reqPayload.groupBy.length > 0
          ) {
            const allBrowsers = sampleBrowserDevices as Array<
              Record<string, unknown>
            >;
            const filterStr =
              typeof reqPayload.filter === 'string' ? reqPayload.filter : '';
            const filtered = filterStr
              ? allBrowsers.filter((b) =>
                  FleetConsoleMockAPI.matchesAip160Filter(b, filterStr),
                )
              : allBrowsers;

            const recordMap = new Map<
              string,
              { dimensions: Record<string, string>; count: number }
            >();
            for (const dev of filtered) {
              const ufsLabels = (dev.ufsLabels || {}) as Record<
                string,
                { values?: string[] }
              >;
              const swLabels = (dev.swarmingLabels || {}) as Record<
                string,
                { values?: string[] }
              >;
              const ufsState =
                (dev.state as string) ||
                ufsLabels['resource_state']?.values?.[0] ||
                'SERVING';

              const swStates = swLabels['state']?.values || [];
              let botHealth = 'MISSING';
              if (swStates.includes('dead')) {
                botHealth = 'DEAD';
              } else if (swStates.includes('quarantined')) {
                botHealth = 'QUARANTINED';
              } else if (swStates.includes('maintenance')) {
                botHealth = 'MAINTENANCE';
              } else if (swStates.includes('alive')) {
                botHealth = 'HEALTHY';
              }

              const dims: Record<string, string> = {};
              for (const g of reqPayload.groupBy as string[]) {
                if (
                  g === 'ufs_labels.resource_state' ||
                  g === 'ufs.resource_state'
                ) {
                  dims[g] = String(ufsState);
                } else if (g === 'state') {
                  dims[g] = String((dev.state as string) || '');
                } else if (g === 'bot_health_status') {
                  dims[g] = botHealth;
                } else {
                  dims[g] = '';
                }
              }

              const key = JSON.stringify(dims);
              const existing = recordMap.get(key);
              if (existing) {
                existing.count++;
              } else {
                recordMap.set(key, { dimensions: dims, count: 1 });
              }
            }

            responseData = {
              total: filtered.length,
              records: Array.from(recordMap.values()).map((r) => ({
                dimensions: r.dimensions,
                count: String(r.count),
              })),
              nextPageToken: '',
            };
          }
        }

        // In-memory AIP-160 filter evaluation & pagination simulation (for static object fixtures)
        if (
          typeof fixture !== 'function' &&
          method !== 'GetFleetAvailabilityTrends'
        ) {
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
              items = items.filter((item) =>
                FleetConsoleMockAPI.matchesAip160Filter(item, filterStr),
              );
              const singleIdMatch =
                method === 'ListDevices' && items.length === 0
                  ? /^id\s*=\s*"?([^"]+)"?$/i.exec(filterStr.trim())
                  : null;
              if (
                singleIdMatch &&
                Array.isArray(sampleChromeosDevices) &&
                sampleChromeosDevices.length > 0
              ) {
                items = [
                  synthesizeChromeosDevice(
                    singleIdMatch[1],
                    sampleChromeosDevices as Array<Record<string, unknown>>,
                  ),
                ];
              }
            }

            const rawLimit =
              typeof reqPayload.pageSize === 'number' && reqPayload.pageSize > 0
                ? reqPayload.pageSize
                : typeof reqPayload.limit === 'number' && reqPayload.limit > 0
                  ? reqPayload.limit
                  : items.length;
            const rawToken =
              typeof reqPayload.pageToken === 'string' && reqPayload.pageToken
                ? reqPayload.pageToken
                : typeof reqPayload.cursor === 'string' && reqPayload.cursor
                  ? reqPayload.cursor
                  : '';
            const startIndex = rawToken.startsWith('token-')
              ? parseInt(rawToken.replace('token-', ''), 10) || 0
              : 0;

            const pagedItems = items.slice(startIndex, startIndex + rawLimit);
            const nextIndex = startIndex + rawLimit;
            const nextPageToken =
              nextIndex < items.length ? `token-${nextIndex}` : '';

            responseData = {
              ...responseData,
              [listArrayKey]: pagedItems,
              nextPageToken,
              ...(listArrayKey === 'items' && nextPageToken
                ? { cursor: nextPageToken }
                : {}),
              totalSize: items.length,
            };
          } else if (method === 'CountDevices' && filterStr) {
            const allCros = sampleChromeosDevices as Array<
              Record<string, unknown>
            >;
            const filteredCounts = computeChromeosCounts(
              allCros.filter((d) =>
                FleetConsoleMockAPI.matchesAip160Filter(d, filterStr),
              ),
            );
            responseData = {
              ...responseData,
              ...filteredCounts,
              chromeosCount: filteredCounts,
              chromeosTotal: filteredCounts.total,
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
