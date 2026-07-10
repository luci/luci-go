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

export const parseChromeosDeviceMetrics = (
  deviceState: DeviceStateCounts | undefined,
  total: number,
) => {
  const ready = deviceState?.ready || 0;
  const needRepair = deviceState?.needRepair || 0;
  const repairFailed = deviceState?.repairFailed || 0;
  const recovering = needRepair + repairFailed;
  const healthy = ready + recovering;
  const needManualRepair = deviceState?.needManualRepair || 0;
  const unhealthy = needManualRepair;
  const needsDeploy = deviceState?.needsDeploy || 0;
  const needsReplacement = deviceState?.needsReplacement || 0;
  const reserved = deviceState?.reserved || 0;

  // 'other' serves as the catch-all category for anything not healthy or unhealthy.
  const other = Math.max(0, total - healthy - unhealthy);
  // 'otherStates' represents the remaining 'other' states after excluding needsDeploy and needsReplacement.
  const otherStates = Math.max(0, other - needsDeploy - needsReplacement);
  // 'otherStatesExcludingReserved' represents 'otherStates' minus reserved.
  const otherStatesExcludingReserved = Math.max(0, otherStates - reserved);

  return {
    total,
    ready,
    needRepair,
    repairFailed,
    recovering,
    healthy,
    needManualRepair,
    unhealthy,
    needsDeploy,
    needsReplacement,
    reserved,
    other,
    otherStates,
    otherStatesExcludingReserved,
  };
};
