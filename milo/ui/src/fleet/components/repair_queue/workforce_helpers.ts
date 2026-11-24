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

import { Duration } from 'luxon';

import { prettySeconds } from '@/fleet/utils/dates';
import { Duration as ProtoDuration } from '@/proto/google/protobuf/duration.pb';

export type TimeframeOption = '1D' | '7D' | '30D' | 'YTD';

/**
 * Customizable copy slots so each platform can tailor Workforce Activity
 * terminology (e.g. "DUT" vs. "device") without branching inside shared UI.
 */
export interface WorkforceActivityLabels {
  readonly inProgressSubtitle: string;
  readonly claimedColumnHeader: string;
  readonly scoreUnit: string;
  readonly completedUnit: string;
  readonly emptyClaimedText: string;
}

export const DEFAULT_WORKFORCE_LABELS: WorkforceActivityLabels = {
  inProgressSubtitle: 'Currently claimed physical DUTs',
  claimedColumnHeader: 'Currently Claimed DUTs (In-Progress)',
  scoreUnit: 'DUT',
  completedUnit: 'devices',
  emptyClaimedText: 'No active devices claimed',
};

/**
 * Platform-neutral claimed task chip rendered in the Workforce Activity table.
 * Structurally compatible with ClaimedAndroidDevice.
 */
export interface WorkforceClaimedDevice {
  readonly deviceId: string;
  readonly score: number;
  readonly duration?: ProtoDuration;
  readonly taskId: string;
}

/**
 * Platform-neutral technician row rendered in the Workforce Activity table.
 * Structurally compatible with AndroidTechnicianActivity.
 */
export interface WorkforceTechnician {
  readonly id: string;
  readonly name: string;
  readonly email: string;
  readonly claimedDevices: readonly WorkforceClaimedDevice[];
  readonly priorityScoreCleared: number;
  readonly avgPtsPerDevice: number;
  readonly avgQueuePickupRank: number;
  readonly completedRepairs: number;
  readonly avgDuration?: ProtoDuration;
}

/**
 * Platform-neutral workforce summary rendered by SharedWorkforceActivityView.
 * Structurally compatible with GetAndroidWorkforceActivityResponse.
 */
export interface WorkforceActivityData {
  readonly activeRepairers: number;
  readonly totalPriorityPointsCleared: number;
  readonly avgQueuePickupRank: number;
  readonly inProgressRepairs: number;
  readonly avgRepairTime?: ProtoDuration;
  readonly technicians: readonly WorkforceTechnician[];
}

export const formatWorkforceDuration = (d?: ProtoDuration): string =>
  d ? prettySeconds(Number(d.seconds)) : '-';

export const formatWorkforceChipDuration = (d?: ProtoDuration): string => {
  if (!d) return '-';
  const seconds = Number(d.seconds);
  if (!seconds) return '0s';

  return Duration.fromObject({ seconds })
    .rescale()
    .toHuman({ unitDisplay: 'short' })
    .split(', ')
    .slice(0, 2)
    .join(', ');
};
