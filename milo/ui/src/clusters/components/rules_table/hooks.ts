// Copyright 2023 The LUCI Authors.
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

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export function useProblemFilterParam(
  validProblemIDs: string[],
): [string, (problemID: string, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  let problemFilter = searchParams.get('problem') || '';

  if (!validProblemIDs.includes(problemFilter)) {
    // Invalid URL parameter. Clear the filter.
    problemFilter = '';
  }

  function updateProblemParam(problemID: string, replace = false) {
    const params = new URLSearchParams();

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'problem') {
        params.set(k, v);
      }
    }

    if (problemID !== '') {
      params.set('problem', problemID);
    }
    setSearchParams(params, {
      replace,
    });
  }

  return [problemFilter, updateProblemParam];
}
