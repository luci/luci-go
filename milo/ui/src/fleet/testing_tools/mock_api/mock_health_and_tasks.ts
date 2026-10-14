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
 * @fileoverview Fleet Health, Swarming/Buildbucket Task History, and UFS
 * MachineLSE fixtures and dynamic request handlers for FleetConsoleMockAPI.
 */

export interface MockHealthSlice {
  id: string;
  name: string;
  filter: string;
  displayOrder: number;
}

const MODEL_TREND_SPECS = [
  {
    name: 'sim-model-nine',
    board: 'mock-board-epsilon',
    pool: 'mock-pool-dev',
    base: 0.942,
    amp: 0.015,
    phase: 0,
    drift: 0.018,
  },
  {
    name: 'demo-model-one',
    board: 'demo-board-alpha',
    pool: 'mock-pool-canary',
    base: 0.924,
    amp: 0.018,
    phase: 3,
    drift: -0.018,
  },
  {
    name: 'demo-model-seven',
    board: 'demo-board-delta',
    pool: 'demo-pool-continuous',
    base: 0.964,
    amp: 0.012,
    phase: 7,
    drift: 0.004,
  },
  {
    name: 'mock-model-two',
    board: 'sim-board-gamma',
    pool: 'sim-pool-performance',
    base: 0.896,
    amp: 0.022,
    phase: 11,
    drift: -0.058,
  },
  {
    name: 'sim-model-three',
    board: 'mock-board-theta',
    pool: 'mock-pool-dev',
    base: 0.934,
    amp: 0.016,
    phase: 15,
    drift: 0.012,
  },
  {
    name: 'sim-model-six',
    board: 'mock-board-epsilon',
    pool: 'mock-pool-canary',
    base: 0.912,
    amp: 0.02,
    phase: 19,
    drift: -0.036,
  },
] as const;

const POOL_TREND_SPECS = [
  {
    name: 'mock-pool-dev',
    board: 'mock-board-epsilon',
    base: 0.936,
    amp: 0.014,
    phase: 0,
    drift: 0.012,
  },
  {
    name: 'mock-pool-canary',
    board: 'demo-board-alpha',
    base: 0.914,
    amp: 0.02,
    phase: 2,
    drift: -0.028,
  },
  {
    name: 'labstation_main',
    board: 'mock-board-labstation',
    base: 0.968,
    amp: 0.01,
    phase: 4,
    drift: 0.005,
  },
  {
    name: 'demo-pool-continuous',
    board: 'demo-board-delta',
    base: 0.892,
    amp: 0.024,
    phase: 9,
    drift: -0.042,
  },
  {
    name: 'sim-pool-performance',
    board: 'sim-board-gamma',
    base: 0.876,
    amp: 0.028,
    phase: 14,
    drift: -0.052,
  },
] as const;

function extractQuotedFilterValues(filterStr: string, key: string): string[] {
  const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const regex = new RegExp(
    `(?:labels\\.)?"?${escaped}"?\\s*(?:=|:)\\s*"?([a-zA-Z0-9_-]+)"?`,
    'gi',
  );
  const matches: string[] = [];
  let match: RegExpExecArray | null = regex.exec(filterStr);
  while (match !== null) {
    matches.push(match[1].toLowerCase());
    match = regex.exec(filterStr);
  }
  return matches;
}

/**
 * Generates schema-valid GetFleetAvailabilityTrendsResponse data that responds
 * to both `grouping` and `filter` parameters.
 */
export function buildMockAvailabilityTrends(
  reqPayload: Record<string, unknown>,
) {
  const rawGrouping = reqPayload?.grouping;
  const isModelGrouping = rawGrouping === 'GROUP_BY_MODEL' || rawGrouping === 2;
  const isPoolGrouping = rawGrouping === 'GROUP_BY_POOL' || rawGrouping === 3;
  const filterStr =
    typeof reqPayload?.filter === 'string' ? reqPayload.filter.trim() : '';

  const pointsCount = 72;
  const hourMs = 3600 * 1000;
  const endMs =
    typeof reqPayload?.endTime === 'string' &&
    !Number.isNaN(Date.parse(reqPayload.endTime))
      ? Math.floor(Date.parse(reqPayload.endTime) / hourMs) * hourMs
      : Math.floor(Date.now() / hourMs) * hourMs;
  const baseMs = endMs - (pointsCount - 1) * hourMs;

  const makePoints = (
    baseGoodRatio: number,
    waveAmp: number,
    phaseOffset: number,
    drift24h = 0,
  ) => {
    const pts: Array<{ timestamp: string; value: number }> = [];
    for (let i = 0; i < pointsCount; i++) {
      const timestamp = new Date(baseMs + i * hourMs).toISOString();
      const wave =
        Math.sin((i + phaseOffset) / 5.5) * waveAmp +
        Math.cos((i + phaseOffset) / 11.0) * (waveAmp * 0.45);
      const drift =
        i >= pointsCount - 25 ? drift24h * ((i - (pointsCount - 25)) / 24) : 0;
      const ratio = Math.min(
        0.99,
        Math.max(0.55, baseGoodRatio + wave + drift),
      );
      pts.push({
        timestamp,
        value: Number(ratio.toFixed(4)),
      });
    }
    return pts;
  };

  const modelFilters = [
    ...extractQuotedFilterValues(filterStr, 'label-model'),
    ...extractQuotedFilterValues(filterStr, 'model'),
  ];
  const poolFilters = [
    ...extractQuotedFilterValues(filterStr, 'label-pool'),
    ...extractQuotedFilterValues(filterStr, 'pool'),
  ];
  const boardFilters = [
    ...extractQuotedFilterValues(filterStr, 'label-board'),
    ...extractQuotedFilterValues(filterStr, 'board'),
  ];

  if (isModelGrouping) {
    const filteredModels = MODEL_TREND_SPECS.filter((m) => {
      if (
        modelFilters.length > 0 &&
        !modelFilters.includes(m.name.toLowerCase())
      ) {
        return false;
      }
      if (
        poolFilters.length > 0 &&
        !poolFilters.includes(m.pool.toLowerCase())
      ) {
        return false;
      }
      if (
        boardFilters.length > 0 &&
        !boardFilters.includes(m.board.toLowerCase())
      ) {
        return false;
      }
      return true;
    });
    const activeModels =
      filteredModels.length > 0 ? filteredModels : MODEL_TREND_SPECS;
    return {
      metricType: 1,
      series: activeModels.map((m) => ({
        name: m.name,
        points: makePoints(m.base, m.amp, m.phase, m.drift),
      })),
    };
  }

  if (isPoolGrouping) {
    const filteredPools = POOL_TREND_SPECS.filter((p) => {
      if (
        poolFilters.length > 0 &&
        !poolFilters.includes(p.name.toLowerCase())
      ) {
        return false;
      }
      if (
        boardFilters.length > 0 &&
        !boardFilters.includes(p.board.toLowerCase())
      ) {
        return false;
      }
      return true;
    });
    const activePools =
      filteredPools.length > 0 ? filteredPools : POOL_TREND_SPECS;
    return {
      metricType: 2,
      series: activePools.map((p) => ({
        name: p.name,
        points: makePoints(p.base, p.amp, p.phase, p.drift),
      })),
    };
  }

  let overallBase = 0.928;
  let overallPhase = 0;
  let overallDrift = 0.008;
  let overallMetricType = 1;
  const matchedModel = MODEL_TREND_SPECS.find((m) =>
    modelFilters.includes(m.name.toLowerCase()),
  );
  if (matchedModel) {
    overallBase = matchedModel.base;
    overallPhase = matchedModel.phase;
    overallDrift = matchedModel.drift;
  } else if (filterStr.includes('mock-pool-dev')) {
    overallBase = 0.946;
    overallPhase = 2;
    overallMetricType = 2;
  } else if (filterStr.includes('labstation')) {
    overallBase = 0.972;
    overallPhase = 5;
    overallMetricType = 2;
  } else if (filterStr.includes('mock-board-epsilon')) {
    overallBase = 0.884;
    overallPhase = 9;
    overallDrift = -0.024;
  }

  return {
    metricType: overallMetricType,
    series: [
      {
        name: 'Overall',
        points: makePoints(overallBase, 0.015, overallPhase, overallDrift),
      },
    ],
  };
}

/**
 * Synthesizes a consistent ChromeOS device for unknown device IDs across both
 * `GetDevice` and single-ID `ListDevices` queries.
 */
export function synthesizeChromeosDevice(
  requestedId: string,
  allCros: ReadonlyArray<Record<string, unknown>>,
): Record<string, unknown> {
  const matched = allCros.find(
    (d) => d.id === requestedId || d.dutId === requestedId,
  );
  if (matched) {
    return JSON.parse(JSON.stringify(matched)) as Record<string, unknown>;
  }
  const fallback = JSON.parse(JSON.stringify(allCros[0] || {})) as Record<
    string,
    unknown
  >;
  const id = requestedId || 'chromeos-device-01';
  fallback.id = id;
  fallback.dutId = id;
  const spec = fallback.deviceSpec as
    | { labels?: Record<string, { values?: string[] }> }
    | undefined;
  if (spec?.labels) {
    spec.labels.id = { values: [id] };
    spec.labels.dut_id = { values: [id] };
    spec.labels.dut_name = { values: [id] };
    spec.labels['label-board'] = { values: ['mock-board-epsilon'] };
    spec.labels['label-model'] = { values: ['sim-model-nine'] };
    spec.labels['label-pool'] = { values: ['mock-pool-dev'] };
  }
  return fallback;
}

export function buildDynamicMachineLse(
  reqPayload: Record<string, unknown>,
  baseLse: Record<string, unknown>,
  allCros: ReadonlyArray<Record<string, unknown>>,
): Record<string, unknown> {
  const rawName = String(reqPayload.name || 'machineLSEs/demo-cros-r2-rk1-h2');
  const host = rawName.replace(/^machineLSEs\//, '');
  const matched =
    allCros.find((d) => d.id === host || d.dutId === host) || allCros[0];
  const labels =
    (
      matched?.deviceSpec as {
        labels?: Record<string, { values?: string[] } | undefined>;
      }
    )?.labels || {};
  const pool = labels['label-pool']?.values?.[0] || 'mock-pool-dev';
  const board = labels['label-board']?.values?.[0] || 'mock-board-epsilon';
  const servoType =
    labels['label-servo_type']?.values?.[0] ||
    'servo_v4p1_with_servo_micro_and_ccd_cr50';
  const baseDut = (
    (baseLse.chromeosMachineLse as Record<string, unknown>)
      ?.deviceLse as Record<string, unknown>
  )?.dut as Record<string, unknown> | undefined;
  const basePeripherals = baseDut?.peripherals as
    | Record<string, unknown>
    | undefined;

  return {
    ...baseLse,
    name: `machineLSEs/${host}`,
    hostname: host,
    osVersion: {
      value: `${board}-release/R134-16181.0.0`,
      description: 'Goldeneye official build',
    },
    chromeosMachineLse: {
      deviceLse: {
        dut: {
          ...baseDut,
          hostname: host,
          pools: [pool],
          peripherals: {
            ...basePeripherals,
            servo: {
              servoHostname: `${host}-labstation1`,
              servoPort: 9999,
              servoSerial: 'SERVOV4P1-S-2409180012',
              servoType,
              servoSetup: 1,
              servoFwChannel: 1,
              usbDrive: {
                serial: 'USB-SANDISK-99102A',
                manufacturer: 'SanDisk Ultra Fit 32GB',
              },
            },
          },
        },
      },
    },
  };
}

export function buildDynamicListBots(
  reqPayload: Record<string, unknown>,
): Record<string, unknown> {
  const dims = Array.isArray(reqPayload.dimensions)
    ? (reqPayload.dimensions as Array<{ key?: string; value?: string }>)
    : [];
  const dutIdDim =
    dims.find((d) => d.key === 'dut_id')?.value || 'demo-cros-r2-rk1-h2';
  return {
    items: [
      {
        botId: `crossk-${dutIdDim}`,
        dimensions: [
          { key: 'dut_id', value: [dutIdDim] },
          { key: 'dut_name', value: [dutIdDim] },
          { key: 'dut_state', value: ['ready'] },
          { key: 'label-board', value: ['mock-board-epsilon'] },
          { key: 'label-model', value: ['sim-model-nine'] },
          { key: 'label-pool', value: ['mock-pool-dev'] },
        ],
        isDead: false,
        quarantined: false,
        maintenanceMsg: '',
      },
    ],
  };
}

export function buildDynamicGetBuild(
  reqPayload: Record<string, unknown>,
  baseBuild: Record<string, unknown>,
): Record<string, unknown> {
  const buildId = String(reqPayload.id || '8741928374918273601');
  const isFailed = buildId.endsWith('03') || buildId.endsWith('06');
  const suiteName = isFailed
    ? 'tast.video-decode-accel-h264'
    : 'tast.critical-system-bvt-inline';
  const chromeosVersion = buildId.endsWith('06')
    ? '16178.0.0'
    : buildId.endsWith('03') || buildId.endsWith('04') || buildId.endsWith('05')
      ? '16180.0.0'
      : '16181.0.0';
  return {
    ...baseBuild,
    id: buildId,
    status: isFailed ? 20 : 12,
    summaryMarkdown: isFailed
      ? `Test suite \`${suiteName}\` failed with 1 unexpected test failure.`
      : `All tests in \`${suiteName}\` passed.`,
    input: {
      properties: {
        $chromeos: {
          ctp2_request: {
            test_request: {
              test_suites: [{ name: suiteName }],
            },
          },
        },
      },
    },
    output: {
      properties: {
        CompressedResult: {
          build_target: 'mock-board-epsilon',
          milestone: '134',
          chromeos_version: chromeosVersion,
          test_suite: suiteName,
          verdict: isFailed ? 'FAILED' : 'PASSED',
        },
      },
    },
  };
}

/**
 * Applies stateful mutations for CreateHealthSlice, UpdateHealthSlice,
 * DeleteHealthSlice, and ReorderHealthSlices directly against the in-memory
 * ListHealthSlices fixture so React Query invalidations preserve user edits.
 */
export function applyHealthSliceMutation(
  method: string,
  reqPayload: Record<string, unknown>,
  fixtures: Record<string, unknown>,
): Record<string, unknown> | null {
  const listFixture = (fixtures.ListHealthSlices as {
    healthSlices?: MockHealthSlice[];
  }) || { healthSlices: [] };
  const currentSlices: MockHealthSlice[] = Array.isArray(
    listFixture.healthSlices,
  )
    ? [...listFixture.healthSlices]
    : [];

  if (method === 'CreateHealthSlice') {
    const incoming = (reqPayload.healthSlice || {}) as Partial<MockHealthSlice>;
    const created: MockHealthSlice = {
      id: incoming.id || `slice-${currentSlices.length + 1}`,
      name: incoming.name || 'New Slice',
      filter: incoming.filter || '',
      displayOrder:
        typeof incoming.displayOrder === 'number'
          ? incoming.displayOrder
          : currentSlices.length,
    };
    fixtures.ListHealthSlices = {
      healthSlices: [...currentSlices, created],
    };
    return { healthSlice: created };
  }

  if (method === 'UpdateHealthSlice') {
    const incoming = (reqPayload.healthSlice || {}) as Partial<MockHealthSlice>;
    const updated: MockHealthSlice = {
      id: incoming.id || 'slice-quota',
      name: incoming.name || 'Updated Slice',
      filter: incoming.filter ?? '',
      displayOrder: incoming.displayOrder ?? 1,
    };
    const exists = currentSlices.some((s) => s.id === updated.id);
    fixtures.ListHealthSlices = {
      healthSlices: exists
        ? currentSlices.map((s) => (s.id === updated.id ? updated : s))
        : [...currentSlices, updated],
    };
    return { healthSlice: updated };
  }

  if (method === 'DeleteHealthSlice') {
    const targetId = String(reqPayload.id || '');
    fixtures.ListHealthSlices = {
      healthSlices: currentSlices.filter((s) => s.id !== targetId),
    };
    return {};
  }

  if (method === 'ReorderHealthSlices') {
    const sliceIds = Array.isArray(reqPayload.sliceIds)
      ? (reqPayload.sliceIds as string[])
      : [];
    const byId = new Map(currentSlices.map((s) => [s.id, s]));
    const reordered: MockHealthSlice[] = [];
    for (const id of sliceIds) {
      const found = byId.get(id);
      if (found) {
        reordered.push({ ...found, displayOrder: reordered.length });
        byId.delete(id);
      }
    }
    for (const remaining of byId.values()) {
      reordered.push({ ...remaining, displayOrder: reordered.length });
    }
    fixtures.ListHealthSlices = { healthSlices: reordered };
    return { healthSlices: reordered };
  }

  return null;
}

const INITIAL_HEALTH_SLICES: MockHealthSlice[] = [
  {
    id: 'all-fleet',
    name: 'All Fleet',
    filter: '',
    displayOrder: 0,
  },
  {
    id: 'slice-quota',
    name: 'DUTs in Dev',
    filter: 'labels."label-pool" = "mock-pool-dev"',
    displayOrder: 1,
  },
  {
    id: 'slice-labstations',
    name: 'Labstations',
    filter:
      'labels."label-pool" = "labstation_main" OR labels."label-pool" = "labstation_tryjob"',
    displayOrder: 2,
  },
  {
    id: 'slice-epsilon',
    name: 'Epsilon Fleet',
    filter: 'labels."label-board" = "mock-board-epsilon"',
    displayOrder: 3,
  },
];

export const HEALTH_AND_TASK_DEFAULT_FIXTURES = {
  ListIrmIncidents: {
    irmIncidents: [
      {
        id: '981421',
        masterBugId: '444677406',
        title:
          'Lab network switch flap in row2-rack4 affecting SSH reachability',
      },
      {
        id: '981305',
        masterBugId: '442819023',
        title:
          'Servo v4p1 firmware update stall on mock-board-epsilon qualification pool',
      },
    ],
  },
  GetFleetAvailabilityTrends: buildMockAvailabilityTrends({ grouping: 0 }),
  ListHealthSlices: {
    healthSlices: INITIAL_HEALTH_SLICES,
  },
  CreateHealthSlice: {
    healthSlice: {
      id: 'slice-new',
      name: 'New Slice',
      filter: '',
      displayOrder: 4,
    },
  },
  UpdateHealthSlice: {
    healthSlice: {
      id: 'slice-quota',
      name: 'Updated Slice',
      filter: '',
      displayOrder: 1,
    },
  },
  DeleteHealthSlice: {},
  ReorderHealthSlices: {
    healthSlices: INITIAL_HEALTH_SLICES,
  },
  ListTaskHistory: {
    tasks: [
      {
        taskId: '68a4f9120bc31010',
        name: 'cros_test_platform: tast.critical-system-bvt-inline (mock-board-epsilon-release/R134-16181.0.0)',
        state: 'COMPLETED',
        startTime: '2026-09-17T14:42:10Z',
        duration: { seconds: '864', nanos: 0 },
        buildVersion: 'R134-16181.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a4f9120bc31010',
        source: 1,
        endTime: '2026-09-17T14:56:34Z',
      },
      {
        taskId: '68a4e38019a24010',
        name: 'AdminRepair: verify-servo-cr50-ec-communication',
        state: 'RUNNING',
        startTime: '2026-09-17T14:58:00Z',
        duration: { seconds: '195', nanos: 0 },
        buildVersion: 'R134-16181.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a4e38019a24010',
        source: 1,
        endTime: undefined,
      },
      {
        taskId: '68a4c1187fe09010',
        name: 'cros_test_platform: tast.video-decode-accel-h264 (mock-board-epsilon-release/R134-16180.0.0)',
        state: 'FAILURE',
        startTime: '2026-09-17T13:18:05Z',
        duration: { seconds: '1142', nanos: 0 },
        buildVersion: 'R134-16180.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a4c1187fe09010',
        source: 1,
        endTime: '2026-09-17T13:37:07Z',
      },
      {
        taskId: '68a49b004dc12010',
        name: 'AdminAudit: storage-smart-health-check & flash-usb-drive',
        state: 'COMPLETED',
        startTime: '2026-09-17T11:50:12Z',
        duration: { seconds: '518', nanos: 0 },
        buildVersion: 'R134-16180.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a49b004dc12010',
        source: 1,
        endTime: '2026-09-17T11:58:50Z',
      },
      {
        taskId: '68a472990ab44010',
        name: 'Deploy: provision-os-image (mock-board-epsilon-release/R134-16180.0.0)',
        state: 'COMPLETED',
        startTime: '2026-09-17T10:12:00Z',
        duration: { seconds: '642', nanos: 0 },
        buildVersion: 'R134-16180.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a472990ab44010',
        source: 1,
        endTime: '2026-09-17T10:22:42Z',
      },
      {
        taskId: '68a440119ce88010',
        name: 'cros_test_platform: power_LoadTest.fast_battery_drain',
        state: 'TIMED_OUT',
        startTime: '2026-09-17T08:00:00Z',
        duration: { seconds: '3600', nanos: 0 },
        buildVersion: 'R134-16178.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a440119ce88010',
        source: 1,
        endTime: '2026-09-17T09:00:00Z',
      },
      {
        taskId: 'mh-job-90812-task-1',
        name: 'MobileHarness: xts-cts-tradefed-demo-arc-suite',
        state: 'COMPLETED',
        startTime: '2026-09-17T06:20:00Z',
        duration: { seconds: '1480', nanos: 0 },
        buildVersion: 'R134-16178.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=mh-job-90812-task-1',
        source: 2,
        endTime: '2026-09-17T06:44:40Z',
      },
      {
        taskId: '68a3f9002ba11010',
        name: 'AdminRepair: servo-power-reset & ec-uart-recovery',
        state: 'COMPLETED',
        startTime: '2026-09-17T04:10:30Z',
        duration: { seconds: '412', nanos: 0 },
        buildVersion: 'R134-16178.0.0',
        taskUrl:
          'https://chromeos-swarming.appspot.com/task?id=68a3f9002ba11010',
        source: 1,
        endTime: '2026-09-17T04:17:22Z',
      },
    ],
    nextPageToken: '',
  },
  ListBots: {
    items: [
      {
        botId: 'crossk-demo-cros-r2-rk1-h2',
        dimensions: [
          { key: 'dut_id', value: ['demo-cros-r2-rk1-h2'] },
          { key: 'dut_name', value: ['demo-cros-r2-rk1-h2'] },
          { key: 'dut_state', value: ['ready'] },
          { key: 'label-board', value: ['mock-board-epsilon'] },
          { key: 'label-model', value: ['sim-model-nine'] },
          { key: 'label-pool', value: ['mock-pool-dev'] },
        ],
        isDead: false,
        quarantined: false,
        maintenanceMsg: '',
      },
    ],
  },
  GetBot: {
    botId: 'crossk-demo-cros-r2-rk1-h2',
    isDead: false,
    quarantined: false,
    maintenanceMsg: '',
  },
  ListBotTasks: {
    items: [
      {
        taskId: '68a4f9120bc31010',
        name: 'bb-8741928374918273601-chromeos/general/Full',
        state: 'COMPLETED',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T14:42:10Z',
        completedTs: '2026-09-17T14:56:34Z',
        duration: 864.2,
        tags: [
          'buildbucket_build_id:8741928374918273601',
          'build:mock-board-epsilon-release/R134-16181.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'swarming.pool:ChromeOSSkylab',
          'label-board:mock-board-epsilon',
          'label-model:sim-model-nine',
          'log_location:logdog://logs.chromium.org/chromeos/buildbucket/cr-buildbucket/8741928374918273601/+/steps/run_test/0/stdout',
        ],
      },
      {
        taskId: '68a4e38019a24010',
        name: 'AdminRepair:demo-cros-r2-rk1-h2',
        state: 'RUNNING',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T14:58:00Z',
        duration: 195.4,
        tags: [
          'buildbucket_build_id:8741928374918273602',
          'build:mock-board-epsilon-release/R134-16181.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'task:recovery',
          'log_location:logdog://logs.chromium.org/chromeos/buildbucket/cr-buildbucket/8741928374918273602/+/steps/repair/0/stdout',
        ],
      },
      {
        taskId: '68a4c1187fe09010',
        name: 'bb-8741928374918273603-chromeos/general/Full',
        state: 'COMPLETED',
        failure: true,
        internalFailure: false,
        startedTs: '2026-09-17T13:18:05Z',
        completedTs: '2026-09-17T13:37:07Z',
        duration: 1142.8,
        tags: [
          'buildbucket_build_id:8741928374918273603',
          'build:mock-board-epsilon-release/R134-16180.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'swarming.pool:ChromeOSSkylab',
          'log_location:logdog://logs.chromium.org/chromeos/buildbucket/cr-buildbucket/8741928374918273603/+/steps/run_test/0/stdout',
        ],
      },
      {
        taskId: '68a49b004dc12010',
        name: 'AdminAudit:demo-cros-r2-rk1-h2',
        state: 'COMPLETED',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T11:50:12Z',
        completedTs: '2026-09-17T11:58:50Z',
        duration: 518.1,
        tags: [
          'buildbucket_build_id:8741928374918273604',
          'build:mock-board-epsilon-release/R134-16180.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'task:audit',
        ],
      },
      {
        taskId: '68a472990ab44010',
        name: 'Deploy:demo-cros-r2-rk1-h2',
        state: 'COMPLETED',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T10:12:00Z',
        completedTs: '2026-09-17T10:22:42Z',
        duration: 642.0,
        tags: [
          'buildbucket_build_id:8741928374918273605',
          'build:mock-board-epsilon-release/R134-16180.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'task:deploy',
        ],
      },
      {
        taskId: '68a440119ce88010',
        name: 'bb-8741928374918273606-chromeos/general/Full',
        state: 'TIMED_OUT',
        failure: true,
        internalFailure: false,
        startedTs: '2026-09-17T08:00:00Z',
        completedTs: '2026-09-17T09:00:00Z',
        duration: 3600.0,
        tags: [
          'buildbucket_build_id:8741928374918273606',
          'build:mock-board-epsilon-release/R134-16178.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'swarming.pool:ChromeOSSkylab',
        ],
      },
      {
        taskId: '68a3f9002ba11010',
        name: 'AdminRepair:demo-cros-r2-rk1-h2',
        state: 'COMPLETED',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T04:10:30Z',
        completedTs: '2026-09-17T04:17:22Z',
        duration: 412.5,
        tags: [
          'buildbucket_build_id:8741928374918273607',
          'build:mock-board-epsilon-release/R134-16178.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'task:recovery',
        ],
      },
      {
        taskId: '68a3b2009dc77010',
        name: 'bb-8741928374918273608-chromeos/general/Full',
        state: 'BOT_DIED',
        failure: false,
        internalFailure: true,
        startedTs: '2026-09-17T02:15:00Z',
        completedTs: '2026-09-17T02:28:14Z',
        duration: 794.0,
        tags: [
          'buildbucket_build_id:8741928374918273608',
          'build:mock-board-epsilon-release/R134-16178.0.0',
          'dut-name:demo-cros-r2-rk1-h2',
          'luci_project:chromeos',
          'swarming.pool:ChromeOSSkylab',
        ],
      },
    ],
  },
  ListTasks: {
    items: [
      {
        taskId: '68a4f9120bc31010',
        name: 'webdriver_headless_shell_wpt / Linux Ubuntu 22.04',
        state: 'COMPLETED',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T14:42:10Z',
        completedTs: '2026-09-17T14:56:34Z',
        duration: 864.2,
      },
      {
        taskId: '68a4e38019a24010',
        name: 'AdminRepair:browser-mtv-row4-rack2-host11',
        state: 'RUNNING',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T14:58:00Z',
        duration: 195.4,
      },
      {
        taskId: '68a4c1187fe09010',
        name: 'telemetry_gpu_integration_test / WebGL2 Conformance',
        state: 'COMPLETED',
        failure: true,
        internalFailure: false,
        startedTs: '2026-09-17T13:18:05Z',
        completedTs: '2026-09-17T13:37:07Z',
        duration: 1142.8,
      },
      {
        taskId: '68a49b004dc12010',
        name: 'interactive_ui_tests / Chrome Desktop Linux',
        state: 'COMPLETED',
        failure: false,
        internalFailure: false,
        startedTs: '2026-09-17T11:50:12Z',
        completedTs: '2026-09-17T11:58:50Z',
        duration: 518.1,
      },
    ],
  },
  GetBuild: {
    id: '8741928374918273601',
    status: 12,
    summaryMarkdown: 'All tests in `tast.critical-system-bvt-inline` passed.',
    input: {
      properties: {
        $chromeos: {
          ctp2_request: {
            test_request: {
              test_suites: [{ name: 'tast.critical-system-bvt-inline' }],
            },
          },
        },
      },
    },
    output: {
      properties: {
        CompressedResult: {
          build_target: 'mock-board-epsilon',
          milestone: '134',
          chromeos_version: '16181.0.0',
          test_suite: 'tast.critical-system-bvt-inline',
          verdict: 'PASSED',
        },
      },
    },
  },
  GetMachineLSE: {
    name: 'machineLSEs/demo-cros-r2-rk1-h2',
    machineLsePrototype: 'atl:standard',
    hostname: 'demo-cros-r2-rk1-h2',
    machines: ['chromeos-asset-0001'],
    osVersion: {
      value: 'mock-board-epsilon-release/R134-16181.0.0',
      description: 'Goldeneye official build',
    },
    updateTime: '2026-09-17T14:30:00Z',
    deploymentTicket: 'b/391048219',
    description: 'Primary ChromeOS hardware qualification DUT',
    resourceState: 3,
    schedulable: true,
    logicalZone: 1,
    zone: 'ZONE_DEMO_DELTA',
    realm: 'chromeos:demo-lab',
    rack: 'demo-cros-r2-rk1',
    chromeosMachineLse: {
      deviceLse: {
        dut: {
          hostname: 'demo-cros-r2-rk1-h2',
          pools: ['mock-pool-dev'],
          hive: 'demo-drone-01',
          peripherals: {
            servo: {
              servoHostname: 'demo-cros-r2-rk1-labstation1',
              servoPort: 9999,
              servoSerial: 'SERVOV4P1-S-2409180012',
              servoType: 'servo_v4p1_with_servo_micro_and_ccd_cr50',
              servoSetup: 1,
              servoFwChannel: 1,
              usbDrive: {
                serial: 'USB-SANDISK-99102A',
                manufacturer: 'SanDisk Ultra Fit 32GB',
              },
            },
            chameleon: {
              chameleonPeripherals: [3, 4],
              audioBoard: false,
            },
            rpm: {
              powerunitName: 'demo-cros-r2-rk1-rpm1',
              powerunitOutlet: 'A14',
              powerunitType: 2,
            },
            wifi: {
              wificell: true,
              antennaConn: 1,
              router: 1,
              wifiRouterFeatures: [1, 2],
            },
            audio: {
              audioBox: false,
              audioCable: true,
              atrus: false,
            },
            cable: [{ type: 1 }, { type: 3 }],
            smartUsbhub: true,
            starfishSlotMapping: 'slot-01:sim-tmobile,slot-02:sim-verizon',
          },
          roVpdMap: {
            serial_number: 'SN-DEMO-ALPHA-0001',
            region: 'us',
            sku_number: '2147483647',
          },
          cbi: {
            rawContents: 'CBIv1:dram_part=LPDDR4X;oem_id=0;sku_id=0x7ffff',
          },
        },
      },
    },
  },
};
