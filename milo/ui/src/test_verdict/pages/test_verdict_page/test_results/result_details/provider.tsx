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

import { useInfiniteQuery } from '@tanstack/react-query';
import { ReactNode, useEffect, useMemo } from 'react';

import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  ListArtifactsRequest,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { parseTestResultName } from '@/test_verdict/tools/utils';

import { ResultDataCtx } from './context';

interface ResultDataProviderProps {
  readonly result: TestResult;
  readonly children: ReactNode;
}

export function ResultDataProvider({
  result,
  children,
}: ResultDataProviderProps) {
  const client = usePrpcServiceClient({
    host: SETTINGS.resultdb.host,
    ClientImpl: ResultDBClientImpl,
  });

  const {
    data: resultArtifactsData,
    error: resultArtifactsError,
    isLoading: resultArtifactsLoading,
    hasNextPage: resultArtifactsHasNextPage,
    fetchNextPage: resultArtifactsFetchNextPage,
  } = useInfiniteQuery({
    ...client.ListArtifacts.queryPaged(
      ListArtifactsRequest.fromPartial({
        parent: result.name,
      }),
    ),
  });

  const {
    data: invArtifactsData,
    error: invArtifactsError,
    isLoading: invArtifactsLoading,
    hasNextPage: invArtifactsHasNextPage,
    fetchNextPage: invArtifactsFetchNextPage,
  } = useInfiniteQuery(
    client.ListArtifacts.queryPaged(
      ListArtifactsRequest.fromPartial({
        parent:
          'invocations/' + parseTestResultName(result.name || '').invocationId,
      }),
    ),
  );

  useEffect(() => {
    if (!resultArtifactsLoading && resultArtifactsHasNextPage) {
      resultArtifactsFetchNextPage();
    }
  }, [
    resultArtifactsFetchNextPage,
    resultArtifactsHasNextPage,
    resultArtifactsLoading,
  ]);

  useEffect(() => {
    if (!invArtifactsLoading && invArtifactsHasNextPage) {
      invArtifactsFetchNextPage();
    }
  }, [invArtifactsFetchNextPage, invArtifactsHasNextPage, invArtifactsLoading]);

  if (resultArtifactsError) {
    throw resultArtifactsError;
  }

  if (invArtifactsError) {
    throw invArtifactsError;
  }

  const resultArtifacts: Artifact[] = useMemo(() => {
    if (
      !resultArtifactsData ||
      resultArtifactsLoading ||
      resultArtifactsHasNextPage
    ) {
      return [];
    }
    return resultArtifactsData.pages.flatMap((p) => p.artifacts);
  }, [resultArtifactsData, resultArtifactsHasNextPage, resultArtifactsLoading]);

  const invArtifacts: Artifact[] = useMemo(() => {
    if (!invArtifactsData || invArtifactsLoading || invArtifactsHasNextPage) {
      return [];
    }
    return invArtifactsData?.pages.flatMap((p) => p.artifacts);
  }, [invArtifactsData, invArtifactsHasNextPage, invArtifactsLoading]);

  return (
    <ResultDataCtx.Provider
      value={{
        result,
        resultArtifacts,
        invArtifacts,
        artifactsLoading: invArtifactsLoading || resultArtifactsLoading,
      }}
    >
      {children}
    </ResultDataCtx.Provider>
  );
}
