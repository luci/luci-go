// Copyright 2024 The LUCI Authors.
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

import { colors } from '@/fleet/theme/colors';

export enum dutState {
  NEEDS_MANUAL_REPAIR = 'NEEDS_MANUAL_REPAIR',
  NEEDS_DEPLOY = 'NEEDS_DEPLOY',
  NEEDS_REPLACEMENT = 'NEEDS_REPLACEMENT',
  RESERVED = 'RESERVED',
  READY = 'READY',
  UNKNOWN = 'UNKNOWN',
  NEEDS_REPAIR = 'NEEDS_REPAIR',
  REPAIR_FAILED = 'REPAIR_FAILED',
}

export const getStatusColor = (status: dutState) => {
  switch (status.toUpperCase()) {
    case dutState.NEEDS_MANUAL_REPAIR:
      return colors.red[100];
    case dutState.NEEDS_DEPLOY:
    case dutState.NEEDS_REPLACEMENT:
      return colors.yellow[100];
    case dutState.RESERVED:
      return colors.purple[100];
    case dutState.READY:
    case dutState.UNKNOWN:
    case dutState.NEEDS_REPAIR:
    case dutState.REPAIR_FAILED:
    default:
      return colors.transparent;
  }
};
