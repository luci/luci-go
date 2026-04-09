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

import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getRow } from './rri_utils';

describe('RRI_COLUMNS', () => {
  it('calculates slippage for incomplete items', () => {
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.IN_PROGRESS,
      resourceRequestTargetDeliveryDate: { year: 2025, month: 3, day: 20 },
      resourceRequestActualDeliveryDate: { year: 2025, month: 3, day: 25 },
      resourceGroups: ['group1', 'group2'],
      acceptedQuantity: BigInt(5),
    } as unknown as ResourceRequest;

    const row = getRow(rr);
    expect(row.slippage).toBe(5);
    expect(row.resource_groups).toBe('group1, group2');
    expect(row.accepted_quantity).toBe('5');
  });

  it('calculates slippage for completed items (LATE)', () => {
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.COMPLETE,
      resourceRequestTargetDeliveryDate: { year: 2025, month: 3, day: 20 },
      resourceRequestActualDeliveryDate: { year: 2025, month: 3, day: 25 },
    } as ResourceRequest;

    const row = getRow(rr);
    expect(row.slippage).toBe(5);
  });

  it('calculates slippage for completed items (EARLY)', () => {
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.COMPLETE,
      resourceRequestTargetDeliveryDate: { year: 2025, month: 3, day: 25 },
      resourceRequestActualDeliveryDate: { year: 2025, month: 3, day: 20 },
    } as ResourceRequest;

    const row = getRow(rr);
    expect(row.slippage).toBe(-5);
  });

  it('calculates 0 slippage if dates are missing', () => {
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.COMPLETE,
    } as ResourceRequest;

    const row = getRow(rr);
    expect(row.slippage).toBe(0);
  });
});
