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

import {
  useSyncedSearchParams,
  searchParamUpdater,
} from '@/generic_libs/hooks/synced_search_params';

const FOLLOW_RENAMES_PARAM_KEY = 'followRenames';

export function useFollowRenames() {
  const [searchParams, setURLSearchParams] = useSyncedSearchParams();
  return [
    searchParams.get(FOLLOW_RENAMES_PARAM_KEY) !== 'false', // Default to true
    (value: boolean) => {
      setURLSearchParams(
        searchParamUpdater(
          FOLLOW_RENAMES_PARAM_KEY,
          value ? 'true' : 'false',
          'true', // default value.
        ),
      );
    },
  ] as [boolean, (val: boolean) => void];
}
