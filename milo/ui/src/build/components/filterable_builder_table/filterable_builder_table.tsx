// Copyright 2024 The LUCI Authors.
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

import { Input } from '@mui/material';
import { useMemo } from 'react';

import { BuilderTable } from '@/build/components/builder_table';
import {
  filterUpdater,
  getFilter,
  getNumOfBuilds,
  numOfBuildsUpdater,
} from '@/build/tools/search_param_utils';
import { SearchInput } from '@/common/components/search_input';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

export interface FilterableBuilderTableProps {
  readonly builders: readonly BuilderID[];
}

export function FilterableBuilderTable({
  builders,
}: FilterableBuilderTableProps) {
  const [searchParams, setSearchparams] = useSyncedSearchParams();
  const numOfBuilds = getNumOfBuilds(searchParams);
  const filter = getFilter(searchParams);

  const buildersWithKeys = useMemo(
    () =>
      builders.map((bid) => {
        return [
          // Pre-compute to support case-insensitive searching.
          `${bid.project}/${bid.bucket}/${bid.builder}`.toLowerCase(),
          bid,
        ] as const;
      }) || [],
    [builders],
  );

  const filteredBuilders = useMemo(() => {
    const parts = filter.toLowerCase().split(' ');
    return buildersWithKeys
      .filter(([lowerBuilderId]) =>
        parts.every((part) => lowerBuilderId.includes(part)),
      )
      .map(([_, builder]) => builder);
  }, [filter, buildersWithKeys]);

  return (
    <>
      <div css={{ margin: '10px' }}>
        <SearchInput
          placeholder="Press '/' to search builders."
          value={filter}
          focusShortcut="/"
          onValueChange={(v) => setSearchparams(filterUpdater(v))}
          initDelayMs={500}
        />
      </div>
      <BuilderTable builders={filteredBuilders} numOfBuilds={numOfBuilds} />
      <div css={{ margin: '5px 5px' }}>
        <div>Showing {builders.length} builders.</div>
        <div>
          Number of builds per builder (10-100):
          <Input
            type="number"
            value={numOfBuilds}
            inputProps={{
              min: 10,
              max: 100,
            }}
            onChange={(e) =>
              setSearchparams(numOfBuildsUpdater(Number(e.target.value)))
            }
          />
        </div>
      </div>
    </>
  );
}
