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

import { useQuery, keepPreviousData } from '@tanstack/react-query';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { GetInvocationRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { GetRootInvocationRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { FetchedArtifactContent } from '../components/artifacts/types';

/**
 * Type of the invocation result.
 * @param data The fetched invocation object, which can be either RootInvocation or a regular Invocation.
 * @param isLegacyInvocation True if the fetched data is a legacy Invocation, false if it is a RootInvocation.
 */
export type InvocationQueryData = {
  data: AnyInvocation;
  isLegacyInvocation: boolean;
};

/**
 * Custom hook to fetch an invocation, automatically trying RootInvocation first,
 * and falling back to a legacy Invocation if the RootInvocation is not found.
 *
 * @param rawInvocationId The unique ID of the invocation (e.g., 'build-8884945112104396289').
 * @param enabled Whether the query should run.
 */
export function useInvocationQuery(
  rawInvocationId: string,
  enabled: boolean = true,
) {
  const resultDbClient = useResultDbClient();

  // 1. Try to fetch RootInvocation
  const rootQueryName = `rootInvocations/${rawInvocationId}`;
  const {
    data: rootInvocation,
    error: rootError,
    isError: isRootError,
    isPending: isRootPending,
  } = useQuery({
    ...resultDbClient.GetRootInvocation.query(
      GetRootInvocationRequest.fromPartial({
        name: rootQueryName,
      }),
    ),
    staleTime: 5 * 60 * 1000,
    enabled,
    retry: false,
  });

  // 2. Fallback to Legacy Invocation if RootInvocation failed
  const legacyQueryName = `invocations/${rawInvocationId}`;
  const {
    data: legacyInvocation,
    error: legacyError,
    isPending: isLegacyPending,
  } = useQuery({
    ...resultDbClient.GetInvocation.query(
      GetInvocationRequest.fromPartial({
        name: legacyQueryName,
      }),
    ),
    staleTime: 5 * 60 * 1000,
    enabled: enabled && isRootError && !rootInvocation,
    placeholderData: keepPreviousData,
    retry: false,
  });

  const invocation: InvocationQueryData | undefined = rootInvocation
    ? {
        data: rootInvocation as AnyInvocation,
        isLegacyInvocation: false,
      }
    : legacyInvocation
      ? {
          data: legacyInvocation as AnyInvocation,
          isLegacyInvocation: true,
        }
      : undefined;

  // Return both errors in an array.
  const errors = [rootError, legacyError].filter(
    (e) => e !== undefined && e !== null,
  ) as Error[];

  return {
    invocation,
    isLoading: isRootPending || (isRootError && isLegacyPending),
    errors,
  };
}

interface Props {
  artifact?: Artifact;
}

export function useFetchArtifactContentQuery({ artifact }: Props) {
  return useQuery<FetchedArtifactContent, Error>({
    queryKey: [
      'artifactContentForSection',
      artifact?.name || 'no-artifact-selected',
    ],
    queryFn: async () => {
      if (!artifact || !artifact.fetchUrl) {
        return {
          data: null,
          isText: false,
          contentType: null,
          error: new Error('Query ran when it should have been disabled'),
          sizeBytes: 0,
        };
      }
      const contentType = artifact.contentType || 'application/octet-stream';
      const isText = contentType.toLowerCase().startsWith('text/');
      try {
        const response = await fetch(artifact.fetchUrl);
        if (!response.ok) {
          throw new Error(
            `Failed to fetch artifact content: ${response.status} ${response.statusText}`,
          );
        }
        if (isText) {
          const textContent = await response.text();
          return {
            data: textContent,
            isText: true,
            contentType,
            error: null,
            sizeBytes: artifact.sizeBytes,
          };
        }
        return {
          data: null,
          isText: false,
          contentType,
          error: null,
          sizeBytes: artifact.sizeBytes,
        };
      } catch (e) {
        return {
          data: null,
          isText: false,
          contentType,
          error: e as Error,
          sizeBytes: artifact.sizeBytes,
        };
      }
    },
    enabled: !!(artifact && artifact.fetchUrl),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
}
