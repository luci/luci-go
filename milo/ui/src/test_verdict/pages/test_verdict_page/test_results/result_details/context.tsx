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

import { useQuery } from '@tanstack/react-query';
import { ReactNode, createContext, useContext } from 'react';

import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  ListArtifactsRequest,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { parseTestResultName } from '@/test_verdict/tools/utils';

interface ResultDataContext {
  readonly result: TestResult;
  readonly resultArtifacts?: readonly Artifact[];
  readonly invArtifacts?: readonly Artifact[];
  artifactsLoading: boolean;
}

export const ResultDataCtx = createContext<ResultDataContext | null>(null);

interface ResultDataProviderProps {
  result: TestResult;
  children: ReactNode;
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
    data: resultArtifacts,
    error: resultArtifactsError,
    isLoading: resultArtifactsLoading,
  } = useQuery({
    ...client.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent: result.name,
      }),
    ),
  });

  const {
    data: invArtifacts,
    error: invArtifactsError,
    isLoading: invArtifactsLoading,
  } = useQuery(
    client.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent:
          'invocations/' + parseTestResultName(result.name || '').invocationId,
      }),
    ),
  );

  if (resultArtifactsError) {
    throw resultArtifactsError;
  }

  if (invArtifactsError) {
    throw invArtifactsError;
  }
  return (
    <ResultDataCtx.Provider
      value={{
        result,
        resultArtifacts: resultArtifacts?.artifacts,
        invArtifacts: invArtifacts?.artifacts,
        artifactsLoading: invArtifactsLoading || resultArtifactsLoading,
      }}
    >
      {children}
    </ResultDataCtx.Provider>
  );
}

export function useResultArtifacts() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useResultArtifacts can only be used in ResultDataProvider',
    );
  }

  return ctx.resultArtifacts;
}

export function useInvArtifacts() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error('useInvArtifacts can only be used in ResultDataProvider');
  }

  return ctx.invArtifacts;
}

export function useCombinedArtifacts() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useCombinedArtifacts can only be used in ResultDataProvider',
    );
  }

  return ctx.resultArtifacts?.concat(ctx.invArtifacts || []) || [];
}

export function useArtifactsLoading() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useArtifactsLoading can only be used in ResultDataProvider',
    );
  }

  return ctx.artifactsLoading;
}

export function useResult() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error('useResult can only be used in ResultDataProvider');
  }

  return ctx.result;
}
