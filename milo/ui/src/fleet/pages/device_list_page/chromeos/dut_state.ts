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

export enum dutState {
  NEEDS_MANUAL_REPAIR = 'NEEDS_MANUAL_REPAIR',
  NEEDS_DEPLOY = 'NEEDS_DEPLOY',
  NEEDS_REPLACEMENT = 'NEEDS_REPLACEMENT',
  RESERVED = 'RESERVED',
  READY = 'READY',
  UNKNOWN = 'UNKNOWN',
  NEEDS_REPAIR = 'NEEDS_REPAIR',
  REPAIR_FAILED = 'REPAIR_FAILED',
  REGISTERED = 'REGISTERED',
}

/**
 * Formats a raw snake_case or SCREAMING_SNAKE_CASE device state string into a
 * human-readable sentence-case label (e.g., "NEEDS_REPAIR" -> "Needs repair",
 * "READY" -> "Ready", "N/A" -> "N/A").
 */
export const formatDeviceStateLabel = (rawState: string): string => {
  const trimmed = rawState.trim();
  if (!trimmed) return '';
  if (trimmed.toUpperCase() === 'N/A') return 'N/A';
  const words = trimmed.toLowerCase().split('_').filter(Boolean);
  if (words.length === 0) return trimmed;
  words[0] = words[0].charAt(0).toUpperCase() + words[0].slice(1);
  return words.join(' ');
};

export const formatDutStateLabel = formatDeviceStateLabel;

export const getStatusColor = (status: StateUnion) => {
  switch (status.toUpperCase()) {
    case dutState.NEEDS_MANUAL_REPAIR:
      return colors.red[100];
    case dutState.NEEDS_DEPLOY:
    case dutState.REGISTERED:
    case dutState.NEEDS_REPLACEMENT:
      return colors.yellow[100];
    case dutState.RESERVED:
      return colors.purple[100];
    case dutState.READY:
      return colors.green[100];
    // Both NEEDS_REPAIR and REPAIR_FAILED are automated recovery states that
    // automation typically recovers from without human intervention; reserve
    // red[100] for NEEDS_MANUAL_REPAIR where physical technician action is required.
    case dutState.NEEDS_REPAIR:
    case dutState.REPAIR_FAILED:
      return colors.orange[100];
    case dutState.UNKNOWN:
      return colors.transparent;
    default:
      return unknownStateColor;
  }
};
