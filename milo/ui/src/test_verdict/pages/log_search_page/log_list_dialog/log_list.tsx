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

import { Button, CircularProgress } from '@mui/material';
import { InfiniteData, useInfiniteQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import {
  ArtifactContentMatcher,
  QueryTestVariantArtifactsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { OutputQueryTestVariantArtifactsResponse } from '@/test_verdict/types';

import { LogGroupIdentifier } from '../contexts';
import { LogSnippetRow } from '../log_snippet_row';

export interface LogListProps {
  readonly project: string;
  readonly logGroupIdentifer: LogGroupIdentifier;
  readonly searchString?: ArtifactContentMatcher;
  readonly startTime: string;
  readonly endTime: string;
}

export function LogList({
  project,
  searchString,
  startTime,
  endTime,
  logGroupIdentifer,
}: LogListProps) {
  const client = useResultDbClient();
  const { testID, variantHash, artifactID } = logGroupIdentifer;
  const {
    data,
    isLoading,
    error,
    isError,
    fetchNextPage,
    isFetchingNextPage,
    hasNextPage,
  } = useInfiniteQuery({
    ...client.QueryTestVariantArtifacts.queryPaged(
      QueryTestVariantArtifactsRequest.fromPartial({
        project,
        searchString,
        testId: testID,
        variantHash,
        artifactId: artifactID,
        startTime,
        endTime,
      }),
    ),
    select: (data) =>
      data as InfiniteData<OutputQueryTestVariantArtifactsResponse>,
  });
  const matchingLogs = useMemo(
    () => data?.pages.flatMap((p) => p.artifacts) || [],
    [data],
  );

  if (isError) {
    throw error;
  }
  if (isLoading) {
    return <CircularProgress sx={{ fontSize: '16px' }} />;
  }
  return (
    <>
      {matchingLogs.map((a) => (
        <LogSnippetRow artifact={a} key={a.name} />
      ))}
      {isFetchingNextPage && <CircularProgress sx={{ font: '14px' }} />}
      {hasNextPage && (
        <Button onClick={() => fetchNextPage()}>Load more</Button>
      )}
    </>
  );
}
