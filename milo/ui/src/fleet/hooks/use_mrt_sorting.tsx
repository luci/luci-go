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

import { MRT_SortingState } from 'material-react-table';
import { useMemo } from 'react';

import { parseOrderByParam } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { OrderByDirection, ORDER_BY_PARAM_KEY } from './order_by';

export function useMrtSorting(): MRT_SortingState {
  const [searchParams] = useSyncedSearchParams();
  const orderByParam = searchParams.get(ORDER_BY_PARAM_KEY);

  return useMemo<MRT_SortingState>(
    () =>
      (orderByParam || '')
        .split(', ')
        .map(parseOrderByParam)
        .filter((x): x is NonNullable<typeof x> => !!x)
        .map((x) => ({
          id: x.field,
          desc: x.direction === OrderByDirection.DESC,
        })),
    [orderByParam],
  );
}
