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

import { useCallback, useMemo } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

const ORDER_BY_PARAM_KEY = 'order_by';

export function useOrderByParam(): [string, (value: string) => void] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const orderByParam = useMemo(
    () => searchParams.get(ORDER_BY_PARAM_KEY) ?? '',
    [searchParams],
  );
  const updateOrderByParam = useCallback(
    (newOrderBy: string) => {
      const newSearchParams = new URLSearchParams(searchParams);
      if (newOrderBy === '') {
        newSearchParams.delete(ORDER_BY_PARAM_KEY);
      } else {
        newSearchParams.set(ORDER_BY_PARAM_KEY, newOrderBy);
      }

      setSearchParams(newSearchParams);
    },
    [searchParams, setSearchParams],
  );

  return [orderByParam, updateOrderByParam];
}
