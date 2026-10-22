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

export const REPAIR_QUEUE_DEFAULT_FIXTURES = {
  ListRepairQueue: {
    repairQueueItems: [
      {
        dutId: 'demo-cros-r2-rk1-h2',
        deviceId: 'demo-cros-r2-rk1-h2',
        pools: ['mock-pool-dev'],
        model: 'sim-model-nine',
        state: 'needs_manual_repair',
        taskId: 'task-001',
        servoState: 'UNKNOWN',
        wifiState: 'UNKNOWN',
        bluetoothState: 'UNKNOWN',
        poolHealthPct: 0.94,
        modelHealthPct: 0.94,
        priorityScore: '100',
        matchedRuleIds: ['b/458449804'],
        etag: 'etag-1',
      },
      {
        dutId: 'demo-cros-r2-rk1-h3',
        deviceId: 'demo-cros-r2-rk1-h3',
        pools: ['mock-pool-dev'],
        model: 'sim-model-nine',
        state: 'needs_manual_repair',
        taskId: 'task-002',
        servoState: 'UNKNOWN',
        wifiState: 'UNKNOWN',
        bluetoothState: 'UNKNOWN',
        poolHealthPct: 0.94,
        modelHealthPct: 0.94,
        priorityScore: '100',
        matchedRuleIds: ['b/458449804'],
        etag: 'etag-2',
      },
      {
        dutId: 'demo-cros-r3-rk2-h1',
        deviceId: 'demo-cros-r3-rk2-h1',
        pools: ['mock-pool-canary'],
        model: 'demo-model-one',
        state: 'needs_manual_repair',
        taskId: 'task-003',
        servoState: 'UNKNOWN',
        wifiState: 'UNKNOWN',
        bluetoothState: 'UNKNOWN',
        poolHealthPct: 0.92,
        modelHealthPct: 0.92,
        priorityScore: '85',
        matchedRuleIds: ['b/542600013'],
        etag: 'etag-3',
      },
      {
        dutId: 'demo-cros-r3-rk2-h2',
        deviceId: 'demo-cros-r3-rk2-h2',
        pools: ['mock-pool-canary'],
        model: 'demo-model-one',
        state: 'needs_manual_repair',
        taskId: 'task-004',
        servoState: 'UNKNOWN',
        wifiState: 'UNKNOWN',
        bluetoothState: 'UNKNOWN',
        poolHealthPct: 0.92,
        modelHealthPct: 0.92,
        priorityScore: '85',
        matchedRuleIds: ['b/542600013'],
        etag: 'etag-4',
      },
      {
        dutId: 'sim-cros-r1-rk3-h4',
        deviceId: 'sim-cros-r1-rk3-h4',
        pools: ['sim-pool-performance'],
        model: 'mock-model-two',
        state: 'repair_failed',
        taskId: 'task-005',
        servoState: 'UNKNOWN',
        wifiState: 'UNKNOWN',
        bluetoothState: 'UNKNOWN',
        poolHealthPct: 0.89,
        modelHealthPct: 0.89,
        priorityScore: '50',
        matchedRuleIds: [],
        etag: 'etag-5',
      },
    ],
    nextPageToken: '',
    totalSize: 5,
    inProgressCount: 0,
  },
  GetSmartRepair: {
    results: [
      {
        deviceId: 'demo-cros-r2-rk1-h2',
        eventId: 'event-smart-repair-1',
      },
      {
        deviceId: 'demo-cros-r2-rk1-h3',
        eventId: 'event-smart-repair-2',
      },
    ],
  },
  ListIrmIncidents: {
    irmIncidents: [
      {
        id: 'irm-001',
        masterBugId: '458449804',
        title: 'OS provision failures on sim-model-nine',
      },
      {
        id: 'irm-002',
        masterBugId: '542600013',
        title: 'Chameleon board disconnected in demo-model-one',
      },
    ],
  },
};
