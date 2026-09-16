#!/usr/bin/env -S node --experimental-strip-types
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
 * @fileoverview Synthetic Mock Data Generator Pipeline for Fleet Console.
 *
 * Generates synthetic mock data for ChromeOS, Android, and Browser fleets,
 * Product Catalog, Repair Metrics, and Resource Requests.
 *
 * All generated names, hostnames, models, boards, zones, and serials use generic
 * synthetic placeholders with explicit "demo-", "mock-", or "sim-" prefixes for
 * local development and offline testing.
 *
 * Usage:
 *   ./src/fleet/scripts/generate_mock_data.ts
 *   node --experimental-strip-types src/fleet/scripts/generate_mock_data.ts
 *   ./src/fleet/scripts/generate_mock_data.ts --full
 *   ./src/fleet/scripts/generate_mock_data.ts --help
 */

/* eslint-disable no-console */

import * as fs from 'node:fs';
import * as path from 'node:path';

// Fixed reference epoch for deterministic mock timestamps (2026-09-01T20:55:04.949Z).
// Never use Date.now() to prevent non-deterministic timestamp churn in git fixtures.
export const DETERMINISTIC_BASE_TIMESTAMP_MS = Date.parse(
  '2026-09-01T20:55:04.949Z',
);

// Seeded pseudo-random number generator (Mulberry32) for reproducible mock sets
export function createPrng(seed: number): () => number {
  let s = seed;
  return function () {
    s |= 0;
    s = (s + 0x6d2b79f5) | 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

let random = createPrng(42);

export function resetSeed(seed = 42): void {
  random = createPrng(seed);
}

function choose<T>(items: readonly T[]): T {
  return items[Math.floor(random() * items.length)];
}

function weightedChoose<T>(items: readonly [T, number][]): T {
  const totalWeight = items.reduce((sum, [, w]) => sum + w, 0);
  let r = random() * totalWeight;
  for (const [item, w] of items) {
    r -= w;
    if (r <= 0) return item;
  }
  return items[0][0];
}

function pad(num: number, digits: number): string {
  return String(num).padStart(digits, '0');
}

// -----------------------------------------------------------------------------
// Type Definitions for Mock Fixtures
// -----------------------------------------------------------------------------

export interface ChromeOSDeviceMock {
  id: string;
  dutId: string;
  state: string;
  type: string;
  address: {
    host: string;
    port: number;
  };
  realm: string;
  deviceSpec: {
    labels: Record<string, { values: readonly string[] | string[] }>;
  };
}

export interface AndroidDeviceMock {
  id: string;
  state: string;
  status: string;
  runTarget: string;
  realm: string;
  fcOfflineSince?: string;
  average7d: number;
  average30d: number;
  omnilabSpec: {
    labels: Record<string, { values: readonly string[] | string[] }>;
  };
}

export interface BrowserDeviceMock {
  id: string;
  realm: string;
  swarmingLabels: Record<string, { values: readonly string[] | string[] }>;
  ufsLabels: Record<string, { values: readonly string[] | string[] }>;
}

export interface ProductCatalogMock {
  id?: string;
  productCatalogId: string;
  gpn: string;
  board?: string;
  model?: string;
  productName: string;
  descriptiveName: string;
  resourceType: string;
  fleetPlmStatus: string;
  r11n: readonly string[] | string[];
  numberOfDevicesPerRack: number;
  unitCost: string;
  productType: string;
}

export interface RepairMetricMock {
  id?: string;
  platform?: string;
  priority: string;
  labName: string;
  hostGroup: string;
  runTarget: string;
  minimumRepairs: number;
  devicesOffline: number;
  totalDevices: number;
  peakUsage: number;
}

export interface ResourceRequestDeliveryDate {
  year: number;
  month: number;
  day: number;
}

export interface ResourceRequestMock {
  id?: string;
  rrId: string;
  resourceRequestBugId: string;
  resourceDetails: string;
  resourceRequestTargetDeliveryDate: ResourceRequestDeliveryDate;
  resourceRequestActualDeliveryDate?: ResourceRequestDeliveryDate;
  procurementTargetDeliveryDate: ResourceRequestDeliveryDate;
  procurementActualDeliveryDate?: ResourceRequestDeliveryDate;
  buildTargetDeliveryDate: ResourceRequestDeliveryDate;
  buildActualDeliveryDate?: ResourceRequestDeliveryDate;
  qaTargetDeliveryDate: ResourceRequestDeliveryDate;
  qaActualDeliveryDate?: ResourceRequestDeliveryDate;
  configTargetDeliveryDate: ResourceRequestDeliveryDate;
  configActualDeliveryDate?: ResourceRequestDeliveryDate;
  fulfillmentStatus: number;
  materialSourcingStatus: number;
  buildStatus: number;
  qaStatus: number;
  configStatus: number;
  customer: string;
  resourceName: string;
  acceptedQuantity: number;
  criticality: string;
  requestApproval: string;
  resourcePm: string;
  fulfillmentChannel: string;
  executionStatus: string;
  resourceGroups: string[];
  resourceRequestStatus: number;
  resourceRequestBugStatus: string;
}

// -----------------------------------------------------------------------------
// Fictional Dictionaries (Obvious Demo Indicators)
// -----------------------------------------------------------------------------

export const FICTIONAL_BOARDS = [
  'demo-board-alpha',
  'mock-board-beta',
  'sim-board-gamma',
  'demo-board-delta',
  'mock-board-epsilon',
  'demo-board-zeta',
  'sim-board-eta',
  'mock-board-theta',
  'demo-board-iota',
  'mock-board-kappa',
  'mock-board-satlab',
] as const;

export const FICTIONAL_MODELS = [
  'demo-model-one',
  'mock-model-two',
  'sim-model-three',
  'demo-model-four',
  'mock-model-five',
  'sim-model-six',
  'demo-model-seven',
  'mock-model-eight',
  'sim-model-nine',
  'demo-model-ten',
] as const;

export const FICTIONAL_LABS = [
  {
    name: 'demo-lab-alpha',
    zone: 'ZONE_DEMO_ALPHA',
    netZone: 'demo-zone-alpha',
  },
  {
    name: 'mock-lab-beta',
    zone: 'ZONE_MOCK_BETA',
    netZone: 'mock-zone-beta',
  },
  {
    name: 'sim-lab-gamma',
    zone: 'ZONE_SIM_GAMMA',
    netZone: 'sim-zone-gamma',
  },
  {
    name: 'demo-lab-delta',
    zone: 'ZONE_DEMO_DELTA',
    netZone: 'demo-zone-delta',
  },
  {
    name: 'sim-lab-epsilon',
    zone: 'ZONE_SIM_EPSILON',
    netZone: 'sim-zone-epsilon',
  },
] as const;

export const FICTIONAL_POOLS = [
  'demo-pool-cq',
  'mock-pool-canary',
  'sim-pool-performance',
  'demo-pool-continuous',
  'mock-pool-dev',
  'sim-pool-unmanaged',
] as const;

export const FICTIONAL_ANDROID_RUN_TARGETS = [
  'demo-target-alpha',
  'mock-target-beta',
  'sim-target-gamma',
  'demo-target-delta',
  'mock-target-epsilon',
  'sim-target-zeta',
] as const;

export const FICTIONAL_ANDROID_HOST_GROUPS = [
  'demo-group-alpha',
  'mock-group-beta',
  'sim-group-gamma',
  'demo-group-delta',
  'mock-group-epsilon',
] as const;

export const FICTIONAL_BROWSER_OS = [
  'Linux',
  'Windows-11',
  'Mac-14.5',
  'Android-14',
] as const;

// -----------------------------------------------------------------------------
// Generator Functions
// -----------------------------------------------------------------------------

export function generateChromeOSDevices(count = 100): ChromeOSDeviceMock[] {
  if (count <= 0) return [];
  const devices: ChromeOSDeviceMock[] = [];

  for (let i = 1; i <= count; i++) {
    const isLabstation = i % 6 === 0;
    const isSatlab = !isLabstation && i % 7 === 0;
    const lab = choose(FICTIONAL_LABS);
    const board = isLabstation
      ? 'mock-board-labstation'
      : isSatlab
        ? 'mock-board-satlab'
        : choose(FICTIONAL_BOARDS);
    const model = isLabstation
      ? 'sim-model-labstation'
      : isSatlab
        ? 'sim-model-satlab'
        : choose(FICTIONAL_MODELS);
    const labstationPool = choose([
      'labstation_main',
      'labstation_canary',
      'labstation_tryjob',
    ]);
    const pool = isLabstation
      ? labstationPool
      : isSatlab
        ? 'satlab-pool'
        : choose(FICTIONAL_POOLS);
    const dutId = isLabstation
      ? `DEMO-LABSTATION-${pad(i, 4)}`
      : isSatlab
        ? `DEMO-SATLAB-${pad(i, 4)}`
        : `DEMO-DUT-${pad(i, 4)}`;
    const row = (i % 10) + 1;
    const rack = (Math.floor(i / 10) % 5) + 1;
    const host = (i % 24) + 1;
    const hostname = isLabstation
      ? `demo-cros-labstation-r${row}-rk${rack}-h${host}`
      : isSatlab
        ? `demo-cros-satlab-r${row}-rk${rack}-h${host}`
        : `demo-cros-r${row}-rk${rack}-h${host}`;

    const dutState = isLabstation
      ? weightedChoose([
          ['ready', 80],
          ['needs_repair', 10],
          ['needs_manual_repair', 5],
          ['repair_failed', 5],
        ])
      : weightedChoose([
          ['ready', 80],
          ['needs_repair', 8],
          ['needs_manual_repair', 6],
          ['repair_failed', 3],
          ['reserved', 3],
        ]);

    const state =
      dutState === 'ready' &&
      weightedChoose([
        ['AVAILABLE', 85],
        ['LEASED', 15],
      ]) === 'LEASED'
        ? 'DEVICE_STATE_LEASED'
        : 'DEVICE_STATE_AVAILABLE';

    const servoState =
      dutState === 'needs_repair' || dutState === 'repair_failed'
        ? weightedChoose([
            ['BROKEN', 50],
            ['NOT_CONNECTED', 30],
            ['NEED_REPLACEMENT', 20],
          ])
        : 'WORKING';

    const servoComponent = choose([
      ['servo_v4p1', 'ccd_cr50'],
      ['servo_v4p1', 'ccd_ti50'],
      ['servo_micro', 'servo_pd'],
      ['servo_v4p1'],
    ]);

    const realm = isSatlab
      ? 'demo:fleet/mock-realm-satlab'
      : 'demo:fleet/mock-realm-cros';

    const labels: Record<string, { values: readonly string[] }> = {
      id: { values: [hostname] },
      dut_id: { values: [dutId] },
      dut_name: { values: [hostname] },
      dut_state: { values: [dutState] },
      'label-board': { values: [board] },
      'label-model': { values: [model] },
      'label-pool': { values: [pool] },
      'label-phase': {
        values: [choose(['PHASE_PVT', 'PHASE_DVT', 'PHASE_EVT'])],
      },
      'label-storage': { values: [choose(['nvme', 'emmc', 'ufs'])] },
      'label-wifi_state': {
        values: [choose(['NORMAL', 'NORMAL', 'NORMAL', 'NO_RESPONSE'])],
      },
      'label-bluetooth_state': { values: ['NORMAL'] },
      'label-bot_size': {
        values: [
          choose(['BOT_SIZE_LARGE', 'BOT_SIZE_MEDIUM', 'BOT_SIZE_SMALL']),
        ],
      },
      'label-cellular_modem_state': {
        values: [choose(['NOT_DETECTED', 'NOT_DETECTED', 'NORMAL'])],
      },
      bot_id: { values: [`demo-crossk-${hostname}`] },
      hwid: { values: [`DEMO-HWID-${board.toUpperCase()}-${pad(i, 4)}`] },
      serial_number: { values: [`DEMO-SN-${pad(i, 6)}`] },
      current_task: { values: [`demo-task-${(100000 + i).toString(16)}`] },
      ufs_zone: { values: [lab.zone] },
      network_zone: { values: [lab.netZone] },
      rack: { values: [`rack-0${rack}`] },
      version_info_os: { values: [`${board}-release/R144-16503.0.0`] },
    };

    if (isLabstation) {
      // Labstations manage DUTs in the rack
      const managedDuts = [
        `demo-cros-r${row}-rk${rack}-h${host + 1}`,
        `demo-cros-r${row}-rk${rack}-h${host + 2}`,
      ];
      labels['label-managed_dut'] = { values: managedDuts };
    } else {
      // DUTs have servo infrastructure and associated labstations
      labels['label-servo_state'] = { values: [servoState] };
      labels['label-servo_usb_state'] = {
        values: [servoState === 'WORKING' ? 'NORMAL' : 'NOT_DETECTED'],
      };
      labels['label-servo_component'] = { values: servoComponent };
      if (!isSatlab) {
        labels['label-associated_hostname'] = {
          values: [`demo-cros-labstation-r${row}-rk${rack}-h1`],
        };
      }
    }

    devices.push({
      id: hostname,
      dutId,
      state,
      type: 'DEVICE_TYPE_PHYSICAL',
      address: {
        host: `${hostname}.lab.demo`,
        port: 22,
      },
      realm,
      deviceSpec: {
        labels,
      },
    });
  }

  return devices;
}

export function generateAndroidDevices(count = 100): AndroidDeviceMock[] {
  if (count <= 0) return [];
  const devices: AndroidDeviceMock[] = [];

  for (let i = 1; i <= count; i++) {
    const lab = choose(FICTIONAL_LABS);
    const model = choose(FICTIONAL_MODELS);
    const runTarget = choose(FICTIONAL_ANDROID_RUN_TARGETS);
    const hostGroup = choose(FICTIONAL_ANDROID_HOST_GROUPS);
    const pool = choose(FICTIONAL_POOLS);
    const deviceId = `DEMO-AND-${pad(i, 4)}`;
    const hostIdx = Math.floor((i - 1) / 4) + 1;
    const hostname = `sim-android-host-${pad(hostIdx, 3)}.${lab.name}.fake`;
    const hostIp = `10.240.${Math.floor(hostIdx / 256)}.${hostIdx % 256 || 1}`;

    const state = weightedChoose([
      ['IDLE', 60],
      ['BUSY', 25],
      ['MISSING', 8],
      ['INIT', 3],
      ['FAILED', 2],
      ['DYING', 1],
      ['LAMEDUCK', 1],
    ]);

    const isOffline = ['MISSING', 'FAILED', 'DYING'].includes(state);
    const offlineSince = isOffline
      ? new Date(DETERMINISTIC_BASE_TIMESTAMP_MS - i * 3600000).toISOString()
      : undefined;

    devices.push({
      id: deviceId,
      state,
      status: state,
      runTarget,
      realm: 'demo:fleet/mock-realm-android',
      fcOfflineSince: offlineSince,
      average7d: Number((0.2 + random() * 0.65).toFixed(3)),
      average30d: Number((0.2 + random() * 0.6).toFixed(3)),
      omnilabSpec: {
        labels: {
          id: { values: [deviceId] },
          control_id: { values: [deviceId] },
          device_class_name: { values: ['AndroidRealDevice'] },
          device_type: {
            values: ['AndroidFlashableDevice', 'AndroidFastbootDevice'],
          },
          model: { values: [model] },
          run_target: { values: [runTarget] },
          host_group: { values: [hostGroup] },
          hostname: { values: [hostname] },
          host_name: { values: [hostname] },
          host_ip: { values: [hostIp] },
          lab_name: { values: [lab.name] },
          lab_location: { values: [lab.name] },
          health_category: {
            values: [
              isOffline
                ? 'HEALTH_CATEGORY_NEED_MANUAL_REPAIR'
                : 'HEALTH_CATEGORY_GOOD',
            ],
          },
          fc_is_offline: { values: [isOffline ? 'true' : 'false'] },
          fc_machine_type: { values: ['device'] },
          dut_state: { values: [isOffline ? 'needs_repair' : 'ready'] },
          state: { values: [state] },
          status: { values: [state] },
          build: { values: ['demo-build-android-15-qpr1'] },
          pool: { values: [pool] },
          type: { values: ['PHYSICAL'] },
          version: { values: ['15.0.0'] },
        },
      },
    });
  }

  return devices;
}

export function generateBrowserDevices(count = 100): BrowserDeviceMock[] {
  if (count <= 0) return [];
  const devices: BrowserDeviceMock[] = [];

  for (let i = 1; i <= count; i++) {
    const lab = choose(FICTIONAL_LABS);
    const os = choose(FICTIONAL_BROWSER_OS);
    const model = choose(FICTIONAL_MODELS);
    const pool = choose(FICTIONAL_POOLS);
    const id = `demo-bot-swarm-${pad(i, 4)}`;
    const rackIdx = Math.floor((i - 1) / 10) + 1;
    const hostname = `demo-browser-host-${pad(i, 4)}.swarm.demo`;

    const state = weightedChoose([
      ['alive', 85],
      ['dead', 7],
      ['quarantined', 5],
      ['maintenance', 3],
    ]);

    const deviceState =
      state === 'alive'
        ? weightedChoose([
            ['available', 80],
            ['busy', 20],
          ])
        : 'error';

    const resourceState =
      state === 'alive'
        ? weightedChoose([
            ['SERVING', 90],
            ['REGISTERED', 10],
          ])
        : weightedChoose([
            ['NEEDS_REPAIR', 60],
            ['MISSING', 20],
            ['REGISTERED', 20],
          ]);

    devices.push({
      id,
      realm: 'demo:fleet/mock-realm-browser',
      swarmingLabels: {
        id: { values: [id] },
        os: { values: [os] },
        pool: { values: [pool] },
        state: { values: [state] },
        device_type: { values: ['demo-bot-desktop'] },
        device_state: { values: [deviceState] },
        device_os: { values: [`${os}-Standard`] },
        dut_state: { values: [state === 'alive' ? 'ready' : 'needs_repair'] },
        current_task: {
          values: [
            state === 'alive' && deviceState === 'busy'
              ? `demo-task-${(200000 + i).toString(16)}`
              : 'idle',
          ],
        },
        last_seen: {
          values: [
            new Date(DETERMINISTIC_BASE_TIMESTAMP_MS - i * 60000).toISOString(),
          ],
        },
        gpu: {
          values: [choose(['none', 'demo-gpu-vulcan', 'mock-gpu-nebula'])],
        },
        zone: { values: [lab.netZone] },
        python: { values: ['3.11.8'] },
        cipd_platform: { values: ['linux-amd64'] },
        cores: { values: [choose(['8', '16', '32'])] },
      },
      ufsLabels: {
        hostname: { values: [hostname] },
        model: { values: [model] },
        rack: { values: [`demo-rack-${pad(rackIdx, 2)}`] },
        zone: { values: [lab.zone] },
        resource_state: { values: [resourceState] },
        schedulable: { values: [state === 'alive' ? 'true' : 'false'] },
        serial_number: { values: [`DEMO-SN-BOT-${pad(i, 5)}`] },
        associated_hostname: {
          values: [`demo-rack-switch-${pad(rackIdx, 2)}`],
        },
        chrome_platform: { values: [os.toLowerCase().split('-')[0]] },
        swarming_instance: { values: ['demo-chromium-swarm'] },
      },
    });
  }

  return devices;
}

export function generateProductCatalog(count = 20): ProductCatalogMock[] {
  if (count <= 0) return [];
  const items: ProductCatalogMock[] = [];
  for (let i = 1; i <= count; i++) {
    const board = FICTIONAL_BOARDS[(i - 1) % FICTIONAL_BOARDS.length];
    const model = FICTIONAL_MODELS[(i - 1) % FICTIONAL_MODELS.length];
    const id = `PROD-CAT-${pad(i, 3)}`;

    items.push({
      productCatalogId: id,
      gpn: `100-${pad(i, 4)}`,
      productName: `Demo ${board.replace('demo-board-', '').replace('mock-board-', '').replace('sim-board-', '')} Pro Laptop`,
      descriptiveName: `Simulated ${model} Testbed Unit (${board})`,
      resourceType: choose(['CHROMEBOOK', 'DESKTOP', 'TABLET', 'CONVERTIBLE']),
      fleetPlmStatus: choose(['ACTIVE', 'ACTIVE', 'ACTIVE', 'EOL', 'PLANNING']),
      r11n: ['DUT_POOL_QUOTA', 'LAB_BENCH'],
      numberOfDevicesPerRack: choose([16, 24, 32]),
      unitCost: `$${(750 + i * 45).toFixed(2)}`,
      productType: 'HARDWARE',
    });
  }
  return items;
}

export function generateRepairMetrics(count = 15): RepairMetricMock[] {
  if (count <= 0) return [];
  const items: RepairMetricMock[] = [];
  for (let i = 1; i <= count; i++) {
    const lab = choose(FICTIONAL_LABS);
    const hostGroup =
      FICTIONAL_ANDROID_HOST_GROUPS[
        (i - 1) % FICTIONAL_ANDROID_HOST_GROUPS.length
      ];
    const runTarget =
      FICTIONAL_ANDROID_RUN_TARGETS[
        (i - 1) % FICTIONAL_ANDROID_RUN_TARGETS.length
      ];
    const priority = weightedChoose([
      ['BREACHED', 30],
      ['WATCH', 40],
      ['NICE', 30],
    ]);
    const totalDevices = choose([12, 16, 24, 32]);
    const offline =
      priority === 'BREACHED'
        ? Math.floor(totalDevices * 0.4)
        : priority === 'WATCH'
          ? Math.floor(totalDevices * 0.2)
          : Math.floor(totalDevices * 0.05);

    items.push({
      priority,
      labName: lab.name,
      hostGroup,
      runTarget,
      minimumRepairs: Math.max(1, Math.floor(offline * 0.7)),
      devicesOffline: offline,
      totalDevices,
      peakUsage: Math.floor(totalDevices * 0.85),
    });
  }
  return items;
}

export function generateResourceRequests(count = 15): ResourceRequestMock[] {
  if (count <= 0) return [];
  const items: ResourceRequestMock[] = [];
  for (let i = 1; i <= count; i++) {
    const board = choose(FICTIONAL_BOARDS);
    const model = choose(FICTIONAL_MODELS);
    const id = `RR-2026-${pad(i, 3)}`;
    let fulfillmentStatus = 1; // IN_PROGRESS
    let materialSourcingStatus = 0;
    let buildStatus = 0;
    let qaStatus = 0;
    let configStatus = 0;
    let executionStatus = 'IN_PROGRESS';

    if (i <= 2) {
      materialSourcingStatus = 1;
    } else if (i <= 5) {
      materialSourcingStatus = 2;
      buildStatus = 1;
    } else if (i <= 7) {
      materialSourcingStatus = 2;
      buildStatus = 2;
      qaStatus = 1;
    } else if (i <= 9) {
      materialSourcingStatus = 2;
      buildStatus = 2;
      qaStatus = 2;
      configStatus = 1;
    } else if (i <= 13) {
      fulfillmentStatus = 2;
      materialSourcingStatus = 2;
      buildStatus = 2;
      qaStatus = 2;
      configStatus = 2;
      executionStatus = 'COMPLETE';
    } else {
      fulfillmentStatus = 0;
      executionStatus = 'NOT_STARTED';
    }

    items.push({
      rrId: id,
      resourceRequestBugId: '0',
      resourceDetails: `${board} ${model} Units (${choose([8, 12, 16, 24])}x)`,
      resourceRequestTargetDeliveryDate: {
        year: 2026,
        month: 6 + (i % 6),
        day: 15,
      },
      resourceRequestActualDeliveryDate:
        fulfillmentStatus === 2
          ? { year: 2026, month: 6 + (i % 6), day: 20 }
          : undefined,
      procurementTargetDeliveryDate: {
        year: 2026,
        month: 6 + (i % 6),
        day: 1,
      },
      procurementActualDeliveryDate:
        materialSourcingStatus >= 2
          ? { year: 2026, month: 6 + (i % 6), day: 3 }
          : undefined,
      buildTargetDeliveryDate: {
        year: 2026,
        month: 6 + (i % 6),
        day: 7,
      },
      buildActualDeliveryDate:
        buildStatus >= 2
          ? { year: 2026, month: 6 + (i % 6), day: 9 }
          : undefined,
      qaTargetDeliveryDate: {
        year: 2026,
        month: 6 + (i % 6),
        day: 12,
      },
      qaActualDeliveryDate:
        qaStatus >= 2 ? { year: 2026, month: 6 + (i % 6), day: 14 } : undefined,
      configTargetDeliveryDate: {
        year: 2026,
        month: 6 + (i % 6),
        day: 15,
      },
      configActualDeliveryDate:
        configStatus >= 2
          ? { year: 2026, month: 6 + (i % 6), day: 20 }
          : undefined,
      fulfillmentStatus,
      materialSourcingStatus,
      buildStatus,
      qaStatus,
      configStatus,
      customer: choose([
        'ChromeOS Test Team',
        'Android Systems',
        'Chromium CI',
        'Infra Performance',
      ]),
      resourceName: `${board}-${model}`,
      acceptedQuantity: choose([8, 12, 16, 24]),
      criticality: choose(['P0', 'P1', 'P2']),
      requestApproval: choose(['APPROVED', 'APPROVED', 'PENDING_APPROVAL']),
      resourcePm: choose([
        'pm-cros@fleet.example.com',
        'pm-android@fleet.example.com',
        'pm-browser@fleet.example.com',
      ]),
      fulfillmentChannel: choose([
        'LAB_ONBOARDING',
        'MH_CELL_DEPLOY',
        'CLOUD_CLUSTER',
      ]),
      executionStatus,
      resourceGroups: ['DUT_POOL_QUOTA'],
      resourceRequestStatus: fulfillmentStatus,
      resourceRequestBugStatus:
        fulfillmentStatus === 2
          ? 'FIXED'
          : fulfillmentStatus === 1
            ? 'ASSIGNED'
            : 'NEW',
    });
  }
  return items;
}

// -----------------------------------------------------------------------------
// CLI Runner & Path Utilities
// -----------------------------------------------------------------------------

export function getDefaultDataDir(): string {
  return path.resolve(process.cwd(), 'src/fleet/testing_tools/mock_api/data');
}

export interface RunGeneratorOptions {
  outputDir?: string;
  isFull?: boolean;
}

/**
 * Safely writes JSON fixtures to disk with strict path traversal prevention (CWE-22).
 * Verifies that filenames cannot escape the designated output directory.
 */
export function writeSafeJson(
  dir: string,
  filename: string,
  data: unknown,
): string {
  const cleanFilename = path.basename(filename);
  if (
    cleanFilename !== filename ||
    filename.includes('..') ||
    filename.includes('/') ||
    filename.includes('\\')
  ) {
    throw new Error(`Path traversal attempt detected in filename: ${filename}`);
  }
  const resolvedDir = path.resolve(dir);
  const targetPath = path.resolve(resolvedDir, cleanFilename);
  if (
    !targetPath.startsWith(resolvedDir + path.sep) &&
    targetPath !== resolvedDir
  ) {
    throw new Error(`Target path escapes destination directory: ${targetPath}`);
  }
  fs.writeFileSync(targetPath, JSON.stringify(data, null, 2) + '\n', 'utf-8');
  return targetPath;
}

export function runGenerator(options: RunGeneratorOptions = {}): string {
  const isFull = options.isFull ?? process.argv.includes('--full');
  const dataDir = options.outputDir
    ? path.resolve(options.outputDir)
    : getDefaultDataDir();

  // Reset seed before generation to guarantee 100% determinism
  resetSeed(42);

  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const chromeosCount = isFull ? 2500 : 20;
  const androidCount = isFull ? 500 : 10;
  const browserCount = isFull ? 200 : 10;
  const catalogCount = isFull ? 24 : 6;
  const repairCount = isFull ? 15 : 3;
  const requestCount = isFull ? 150 : 5;

  console.log(
    `[GENERATOR] Generating synthetic Fleet Console mock data in: ${dataDir} (${isFull ? 'full scale' : 'sample fixtures'})`,
  );

  const chromeos = generateChromeOSDevices(chromeosCount);
  const android = generateAndroidDevices(androidCount);
  const browser = generateBrowserDevices(browserCount);
  const catalog = generateProductCatalog(catalogCount);
  const repairs = generateRepairMetrics(repairCount);
  const requests = generateResourceRequests(requestCount);

  writeSafeJson(dataDir, 'chromeos_devices.json', chromeos);
  writeSafeJson(dataDir, 'android_devices.json', android);
  writeSafeJson(dataDir, 'browser_devices.json', browser);
  writeSafeJson(dataDir, 'product_catalog.json', catalog);
  writeSafeJson(dataDir, 'repair_metrics.json', repairs);
  writeSafeJson(dataDir, 'resource_requests.json', requests);

  console.log('[SUCCESS] Generated sample JSON fixtures:');
  const printSize = (filename: string, label: string, count: number) => {
    const filePath = path.join(dataDir, filename);
    const sizeKb = (fs.statSync(filePath).size / 1024).toFixed(1);
    console.log(`  - ${count} ${label} (${sizeKb} KB)`);
  };
  printSize('chromeos_devices.json', 'ChromeOS Devices', chromeosCount);
  printSize('android_devices.json', 'Android Devices ', androidCount);
  printSize('browser_devices.json', 'Browser Devices ', browserCount);
  printSize('product_catalog.json', 'Product Catalog  ', catalogCount);
  printSize('repair_metrics.json', 'Repair Metrics   ', repairCount);
  printSize('resource_requests.json', 'Resource Requests', requestCount);

  return dataDir;
}

export function handleCli(): void {
  if (process.argv.includes('--help') || process.argv.includes('-h')) {
    console.log(`Usage:
  ./src/fleet/scripts/generate_mock_data.ts [options]

Options:
  --full       Generate full scale mock dataset for UI performance testing
  --help, -h   Show this help message
`);
    return;
  }

  const validFlags = new Set(['--full', '--help', '-h']);
  const unknownFlags = process.argv
    .slice(2)
    .filter((arg) => arg.startsWith('-') && !validFlags.has(arg));
  if (unknownFlags.length > 0) {
    console.error(`[ERROR] Unknown flag(s): ${unknownFlags.join(', ')}`);
    console.error('Run with --help to see available options.');
    process.exitCode = 1;
    return;
  }

  runGenerator();
}

const isDirectExecution = Boolean(
  process.argv[1] &&
    process.argv[1].endsWith('generate_mock_data.ts') &&
    !process.env.JEST_WORKER_ID,
);

if (isDirectExecution) {
  handleCli();
}
