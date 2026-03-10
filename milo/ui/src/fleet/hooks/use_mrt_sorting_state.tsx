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

import { MRT_SortingState, MRT_Updater } from 'material-react-table';
import { useCallback } from 'react';

import { useOrderByParam } from './order_by';
import { useMrtSorting } from './use_mrt_sorting';

export function useMrtSortingState(): [
  MRT_SortingState,
  (updater: MRT_Updater<MRT_SortingState>) => void,
] {
  const sorting = useMrtSorting();
  const [, updateOrderByParam] = useOrderByParam();

  const onSortingChange = useCallback(
    (updater: MRT_Updater<MRT_SortingState>) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;

      updateOrderByParam(
        newSorting.map((s) => (s.desc ? `${s.id} desc` : s.id)).join(', '),
      );
    },
    [sorting, updateOrderByParam],
  );

  return [sorting, onSortingChange];
}
