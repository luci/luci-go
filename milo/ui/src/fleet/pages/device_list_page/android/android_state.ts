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
import { colors, unknownStateColor } from '@/fleet/theme/colors';

export enum androidState {
  BUSY = 'BUSY',
  DYING = 'DYING',
  IDLE = 'IDLE',
  INIT = 'INIT',
  LAB_MISSING = 'LAB_MISSING',
  LAB_RUNNING = 'LAB_RUNNING',
  LAMEDUCK = 'LAMEDUCK',
  MISSING = 'MISSING',
  PREPPING = 'PREPPING',
}

export const getAndroidStatusColor = (status: StateUnion) => {
  switch (status.toUpperCase()) {
    case androidState.DYING:
    case androidState.LAB_RUNNING:
    case androidState.MISSING:
      return colors.red[100];
    case androidState.PREPPING:
    case androidState.INIT:
      return colors.yellow[100];
    case androidState.IDLE:
    case androidState.BUSY:
    case androidState.LAMEDUCK:
    case androidState.LAB_MISSING:
      return colors.transparent;
    default:
      return unknownStateColor;
  }
};
