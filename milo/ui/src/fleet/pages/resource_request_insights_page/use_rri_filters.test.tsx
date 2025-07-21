// Copyright 2025 The LUCI Authors.
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

// eslint-disable-next-line max-len
import { GetResourceRequestsMultiselectFilterValuesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { getSortedMultiselectElements } from './use_rri_filters';

const MOCK_FILTER_VALUES =
  GetResourceRequestsMultiselectFilterValuesResponse.fromPartial({
    rrIds: ['rr-id-1', 'rr-id-2', 'abc-123', 'xyz-123'],
  });

describe('getSortedMultiselectElements', () => {
  it('should perform fuzzy sort based on search query', () => {
    const result = getSortedMultiselectElements(
      MOCK_FILTER_VALUES,
      'rr_id',
      '123',
    );
    expect(result.map((r) => r.el.value)).toEqual([
      'abc-123',
      'xyz-123',
      'rr-id-1',
      'rr-id-2',
    ]);
    expect(result[0].score).toBeGreaterThan(result[2].score);
  });

  it('should boost initial selections to the top', () => {
    const result = getSortedMultiselectElements(
      MOCK_FILTER_VALUES,
      'rr_id',
      '',
      ['rr-id-2', 'xyz-123'],
    );
    expect(result.map((r) => r.el.value)).toEqual([
      'rr-id-2',
      'xyz-123',
      'abc-123',
      'rr-id-1',
    ]);
  });

  it('should boost initial selections to the top with search query', () => {
    const result = getSortedMultiselectElements(
      MOCK_FILTER_VALUES,
      'rr_id',
      '1',
      ['rr-id-2', 'xyz-123'],
    );
    expect(result.map((r) => r.el.value)).toEqual([
      'xyz-123',
      'abc-123',
      'rr-id-1',
      'rr-id-2',
    ]);
  });

  it('should handle empty search query', () => {
    const result = getSortedMultiselectElements(
      MOCK_FILTER_VALUES,
      'rr_id',
      '',
    );
    expect(result.map((r) => r.el.value)).toEqual([
      'abc-123',
      'rr-id-1',
      'rr-id-2',
      'xyz-123',
    ]);
  });

  it('should return empty array for unknown option', () => {
    const result = getSortedMultiselectElements(
      MOCK_FILTER_VALUES,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      'non_existent_key' as any,
      '',
    );
    expect(result).toEqual([]);
  });
});
