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

import { StateUnion } from '@/fleet/components/table/cell_with_chip';
import { fleetColors, toneColors } from '@/fleet/theme/tokens';

export enum swarmingState {
  DEAD = 'DEAD',
  QUARANTINED = 'QUARANTINED',
  MAINTENANCE = 'MAINTENANCE',
  ALIVE = 'ALIVE',
}

export const getBrowserSwarmingStateColor = (status: StateUnion) => {
  switch (status.toUpperCase()) {
    case swarmingState.DEAD:
      return toneColors.error.container;
    case swarmingState.QUARANTINED:
      return toneColors.warning.container;
    case swarmingState.MAINTENANCE:
      return toneColors.teal.container;
    case swarmingState.ALIVE:
      return toneColors.success.container;
    default:
      return fleetColors.unknownState;
  }
};

export const sortSwarmingStates = (stateValues: string[]) => {
  const sortOrder: string[] = [
    swarmingState.DEAD,
    swarmingState.QUARANTINED,
    swarmingState.MAINTENANCE,
  ];

  stateValues.sort((a, b) => {
    const indexA = sortOrder.indexOf(a);
    const indexB = sortOrder.indexOf(b);
    if (indexA !== -1 && indexB !== -1) return indexA - indexB;
    if (indexA !== -1) return -1;
    if (indexB !== -1) return 1;
    return a.localeCompare(b);
  });

  return stateValues;
};
