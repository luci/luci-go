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

import { OptionValue } from '@/fleet/types/option';
import { SortedElement, fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { ResourceRequest_Status } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const fulfillmentStatusDisplayValueMap: Record<
  keyof typeof ResourceRequest_Status,
  string
> = {
  NOT_STARTED: 'Not Started',
  IN_PROGRESS: 'In Progress',
  COMPLETED: 'Completed',
};

export const getFulfillmentStatusScoredOptions = (
  searchQuery: string,
): SortedElement<OptionValue>[] => {
  return fuzzySort(searchQuery)(
    (
      Object.keys(
        fulfillmentStatusDisplayValueMap,
      ) as (keyof typeof ResourceRequest_Status)[]
    ).map(
      (s) =>
        ({
          value: s,
          label: fulfillmentStatusDisplayValueMap[s],
        }) as OptionValue,
    ),
    (o) => o.label,
  );
};
