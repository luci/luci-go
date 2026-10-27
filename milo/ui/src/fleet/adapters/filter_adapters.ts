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

import { remapFilterDimensions } from '@/fleet/utils/aip160/transform';

/**
 * Mapping of ChromeOS Health Dashboard dimension keys to Devices List filter keys.
 * Health queries aggregate by top-level dimensions (model, pool, device_id),
 * whereas Devices List inventory queries require label prefixes (labels.model, labels.pool, id).
 */
export const HEALTH_TO_DEVICE_DIMENSION_MAP: Record<string, string> = {
  model: 'labels."label-model"',
  pool: 'labels."label-pool"',
  device_id: 'id',
};

/**
 * Converts a Health Dashboard / Slice filter to a Devices List filter using AIP-160 AST remapping.
 *
 * NOTE: When CL 8545517 (branded filter types) is merged, this function's signature
 * will transition to: `(healthFilter: HealthFilter): DeviceFilter`.
 */
export function healthToDeviceFilter(filter?: string | null): string {
  if (!filter || !filter.trim()) return '';
  return remapFilterDimensions(filter, HEALTH_TO_DEVICE_DIMENSION_MAP);
}
