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
import { useEffect, useState } from 'react';

import { useFeatureFlag } from '@/common/feature_flags/context';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { GetInvocationRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { GetRootInvocationRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { FetchedArtifactContent } from '../components/artifacts/types';
import { USE_ROOT_INVOCATION_FLAG } from '../pages/features';

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
  const useRootInvocation = useFeatureFlag(USE_ROOT_INVOCATION_FLAG);

  const [useFallback, setUseFallback] = useState(false);

  // Reset fallback if parameters change.
  useEffect(() => {
    setUseFallback(false);
  }, [rawInvocationId, useRootInvocation, enabled]);

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

    enabled: enabled && (useRootInvocation || useFallback),
    retry: false,
  });

  const legacyQueryName = `invocations/${rawInvocationId}`;
  const {
    data: legacyInvocation,
    error: legacyError,
    isError: isLegacyError,
    isPending: isLegacyPending,
  } = useQuery({
    ...resultDbClient.GetInvocation.query(
      GetInvocationRequest.fromPartial({
        name: legacyQueryName,
      }),
    ),
    staleTime: 5 * 60 * 1000,

    enabled: enabled && (!useRootInvocation || useFallback),
    placeholderData: keepPreviousData,
    retry: false,
  });

  // Trigger fallback if the primary fails.
  useEffect(() => {
    if (useRootInvocation && isRootError && !rootInvocation) {
      setUseFallback(true);
    }
    if (!useRootInvocation && isLegacyError && !legacyInvocation) {
      setUseFallback(true);
    }
  }, [
    useRootInvocation,
    isRootError,
    rootInvocation,
    isLegacyError,
    legacyInvocation,
  ]);

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

  const errors = [rootError, legacyError].filter(
    (e) => e !== undefined && e !== null,
  ) as Error[];

  return {
    invocation,
    isLoading: useRootInvocation
      ? isRootPending || (isRootError && isLegacyPending)
      : isLegacyPending || (isLegacyError && isRootPending),
    errors,
  };
}

interface Props {
  artifact?: Artifact;
}

export function useFetchArtifactContentQuery({
  artifact,
  limit,
  enabled = true,
}: Props & { limit?: number; enabled?: boolean }) {
  return useQuery<FetchedArtifactContent, Error>({
    queryKey: [
      'artifactContentForSection',
      artifact?.name || 'no-artifact-selected',
      limit,
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
        const url = new URL(artifact.fetchUrl);
        const headers: HeadersInit = {};
        if (limit) {
          // If the URL is a GCS signed URL (which we can guess by the presence of X-Goog-Signature),
          // we cannot add query parameters as it will invalidate the signature.
          // Instead, we use the Range header.
          // We also check for storage.googleapis.com to be safe, although signed URLs can be on other domains too.
          const isGcsSignedUrl =
            url.searchParams.has('X-Goog-Signature') ||
            url.hostname === 'storage.googleapis.com';

          if (isGcsSignedUrl) {
            headers['Range'] = `bytes=0-${limit - 1}`;
          } else {
            url.searchParams.set('n', limit.toString());
          }
        }
        const response = await fetch(url.toString(), { headers });
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
    enabled: !!(artifact && artifact.fetchUrl) && enabled,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
}
