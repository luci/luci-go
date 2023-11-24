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

import { useMemo } from 'react';

import { usePrpcQueries } from '@/common/hooks/legacy_prpc_query';
import {
  Build,
  BuildsService,
  SEARCH_BUILD_FIELD_MASK,
} from '@/common/services/buildbucket';

import { RelatedBuildTable } from './related_build_table';

export interface RelatedBuildsDisplayProps {
  readonly build: Pick<Build, 'tags'>;
}

export function RelatedBuildsDisplay({ build }: RelatedBuildsDisplayProps) {
  const buildsets =
    build.tags?.filter(
      (t) =>
        t.key === 'buildset' &&
        // Remove the commit/git/ buildsets because we know they're redundant
        // with the commit/gitiles/ buildsets, and we don't need to ask
        // Buildbucket twice.
        !t.value?.startsWith('commit/git/'),
    ) || [];

  const responses = usePrpcQueries({
    host: SETTINGS.buildbucket.host,
    Service: BuildsService,
    method: 'searchBuilds',
    requests: buildsets.map((tag) => ({
      predicate: { tags: [tag] },
      fields: SEARCH_BUILD_FIELD_MASK,
      pageSize: 1000,
    })),
  });

  for (const res of responses) {
    if (res.isError) {
      throw res.error;
    }
  }

  const isLoading = responses.some((res) => res.isLoading);
  const relatedBuilds = useMemo(() => {
    if (isLoading) {
      return [];
    }

    const buildMap = new Map<string, Build>();
    for (const res of responses) {
      for (const build of res.data?.builds || []) {
        // Filter out duplicate builds by overwriting them.
        buildMap.set(build.id, build);
      }
    }
    const builds = [...buildMap.values()].sort((b1, b2) =>
      b1.id.length === b2.id.length
        ? b1.id.localeCompare(b2.id)
        : b1.id.length - b2.id.length,
    );
    return builds;
  }, [isLoading, responses]);

  if (!isLoading && relatedBuilds.length === 0) {
    return (
      <div css={{ padding: '10px 20px' }}>
        No other builds found with the same buildset
      </div>
    );
  }

  return (
    <>
      <div css={{ padding: '0px 20px' }}>
        <h4>Other builds with the same buildset</h4>
        <ul>
          {buildsets.map(({ value }) => (
            <li key={value}>{value}</li>
          ))}
        </ul>
      </div>
      <RelatedBuildTable
        relatedBuilds={relatedBuilds}
        showLoadingBar={isLoading}
      />
    </>
  );
}
