// Copyright 2022 The LUCI Authors.
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

import { groupBy, mapValues } from 'lodash-es';
import { useEffect, useMemo } from 'react';

import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { MiloInternal } from '@/common/services/milo_internal';

import { BuilderListDisplay } from './builder_list_display';

interface BuilderListProps {
  readonly searchQuery: string;
}

export function BuilderList({ searchQuery }: BuilderListProps) {
  const { data, isError, error, isLoading, fetchNextPage, hasNextPage } =
    useInfinitePrpcQuery({
      host: '',
      insecure: location.protocol === 'http:',
      Service: MiloInternal,
      method: 'listBuilders',
      request: {
        pageSize: 10000,
      },
    });

  if (isError) {
    throw error;
  }

  // Computes `builders` separately so it's not re-computed when only
  // `searchQuery` is updated.
  const builders = useMemo(
    () =>
      data?.pages
        .flatMap((p) => p.builders || [])
        .map((b) => {
          return [
            // Pre-compute to support case-sensitive grouping.
            `${b.id.project}/${b.id.bucket}`,
            // Pre-compute to support case-insensitive searching.
            `${b.id.project}/${b.id.bucket}/${b.id.builder}`.toLowerCase(),
            b.id,
          ] as const;
        }) || [],
    [data],
  );

  // Filter & group builders.
  const groupedBuilders = useMemo(() => {
    const parts = searchQuery.toLowerCase().split(' ');
    const filteredBuilders = builders.filter(([_, lowerBuilderId]) =>
      parts.every((part) => lowerBuilderId.includes(part)),
    );
    return mapValues(
      groupBy(filteredBuilders, ([bucketId]) => bucketId),
      (builders) =>
        builders.map(([_bucketId, _lowerBuilderId, builder]) => builder),
    );
  }, [builders, searchQuery]);

  // Keep loading builders until all pages are loaded.
  useEffect(() => {
    if (!isLoading && hasNextPage) {
      fetchNextPage();
    }
  }, [fetchNextPage, isLoading, hasNextPage, data?.pages.length]);

  return (
    <BuilderListDisplay
      groupedBuilders={groupedBuilders}
      isLoading={isLoading}
    />
  );
}
