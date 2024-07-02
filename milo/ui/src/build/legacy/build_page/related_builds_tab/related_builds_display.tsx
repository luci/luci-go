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

import { Checkbox, FormControlLabel, FormGroup, styled } from '@mui/material';
import { UseQueryOptions, useQueries } from '@tanstack/react-query';
import { useState } from 'react';

import { PARTIAL_BUILD_FIELD_MASK } from '@/build/constants';
import { OutputBuild } from '@/build/types';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import {
  CategoryTree,
  CategoryTreeEntry,
} from '@/generic_libs/tools/category_tree';
import {
  BuildsClientImpl,
  SearchBuildsRequest,
  SearchBuildsResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { RelatedBuildTable } from './related_build_table';

const FilterGroup = styled(FormGroup)`
  & .MuiCheckbox-root {
    padding: 5px 5px 5px 10px;
  }
`;

export interface RelatedBuildsDisplayProps {
  readonly build: OutputBuild;
}

export function RelatedBuildsDisplay({ build }: RelatedBuildsDisplayProps) {
  const buildsets =
    build.tags.filter(
      (t) =>
        t.key === 'buildset' &&
        // Remove the commit/git/ buildsets because we know they're redundant
        // with the commit/gitiles/ buildsets, and we don't need to ask
        // Buildbucket twice.
        !t.value?.startsWith('commit/git/'),
    ) || [];

  const [selectedBuildTree, setSelectedBuildTree] = useState(true);
  const [unselectedBuildSets, setUnselectedBuildSets] = useState(
    new Set<string>(),
  );

  const client = usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
  });
  type QueryOpts = UseQueryOptions<SearchBuildsResponse>;
  const rootBuildId = build.ancestorIds.length
    ? build.ancestorIds[0]
    : build.id;
  const queries = useQueries({
    queries: [
      {
        ...client.SearchBuilds.query(
          SearchBuildsRequest.fromPartial({
            predicate: {
              build: { startBuildId: rootBuildId, endBuildId: rootBuildId },
            },
            mask: {
              fields: PARTIAL_BUILD_FIELD_MASK,
            },
          }),
        ),
        // We only need to query the root build if it's not the same as the
        // current build.
        enabled: selectedBuildTree && rootBuildId !== build.id,
      },
      {
        ...client.SearchBuilds.query(
          SearchBuildsRequest.fromPartial({
            predicate: {
              descendantOf: rootBuildId,
            },
            mask: {
              fields: PARTIAL_BUILD_FIELD_MASK,
            },
            pageSize: 1000,
          }),
        ),
        enabled: selectedBuildTree,
      },
      ...buildsets.map((tag) => ({
        ...client.SearchBuilds.query(
          SearchBuildsRequest.fromPartial({
            predicate: { tags: [tag] },
            mask: {
              fields: PARTIAL_BUILD_FIELD_MASK,
            },
            pageSize: 1000,
          }),
        ),
        enabled: !unselectedBuildSets.has(tag.value),
      })),
    ].filter((q: QueryOpts) => q.enabled ?? true),
  });

  for (const query of queries) {
    if (query.isError) {
      throw query.error;
    }
  }

  const isLoading = queries.some((query) => query.isLoading);
  const builds: CategoryTreeEntry<string, OutputBuild>[] = [
    [[...build.ancestorIds, build.id], build],
  ];
  for (const query of queries) {
    builds.push(
      ...(query.data?.builds.map(
        (b) => [[...b.ancestorIds, b.id], b as OutputBuild] as const,
      ) || []),
    );
  }
  const buildTree = new CategoryTree(builds);

  return (
    <>
      <div css={{ padding: '0px 20px' }}>
        <h4 css={{ marginBottom: '10px' }}>Show Builds:</h4>
        <FilterGroup>
          <FormControlLabel
            control={
              <Checkbox
                checked={selectedBuildTree}
                onChange={(_, checked) => setSelectedBuildTree(checked)}
                size="small"
              />
            }
            label="in the same build tree"
          />
          {buildsets.map(({ value }) => (
            <FormControlLabel
              key={value}
              control={
                <Checkbox
                  checked={!unselectedBuildSets.has(value)}
                  onChange={(_, checked) =>
                    setUnselectedBuildSets((oldUnselected) => {
                      const newUnselected = new Set(oldUnselected);
                      if (checked) {
                        newUnselected.delete(value);
                      } else {
                        newUnselected.add(value);
                      }
                      return newUnselected;
                    })
                  }
                  size="small"
                />
              }
              label={`with buildset: "${value}"`}
            />
          ))}
        </FilterGroup>
      </div>
      <RelatedBuildTable
        selfBuild={build}
        buildTree={buildTree}
        showLoadingBar={isLoading}
      />
    </>
  );
}
