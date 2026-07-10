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

import { DeviceStateCounts } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { parseChromeosDeviceMetrics } from './metrics';

describe('parseChromeosDeviceMetrics', () => {
  it('should calculate metrics correctly when all states are set', () => {
    const deviceState: DeviceStateCounts = {
      ready: 10,
      needRepair: 3,
      repairFailed: 2,
      needManualRepair: 1,
      needsDeploy: 1,
      needsReplacement: 1,
      reserved: 1,
    };
    const result = parseChromeosDeviceMetrics(deviceState, 20);
    expect(result).toEqual({
      total: 20,
      ready: 10,
      needRepair: 3,
      repairFailed: 2,
      recovering: 5,
      healthy: 15,
      needManualRepair: 1,
      unhealthy: 1,
      needsDeploy: 1,
      needsReplacement: 1,
      reserved: 1,
      other: 4,
      otherStates: 2,
      otherStatesExcludingReserved: 1,
    });
  });

  it('should handle undefined deviceState gracefully', () => {
    const result = parseChromeosDeviceMetrics(undefined, 10);
    expect(result).toEqual({
      total: 10,
      ready: 0,
      needRepair: 0,
      repairFailed: 0,
      recovering: 0,
      healthy: 0,
      needManualRepair: 0,
      unhealthy: 0,
      needsDeploy: 0,
      needsReplacement: 0,
      reserved: 0,
      other: 10,
      otherStates: 10,
      otherStatesExcludingReserved: 10,
    });
  });
});
