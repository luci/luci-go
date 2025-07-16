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

import { useQuery } from '@tanstack/react-query';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { FetchedArtifactContent } from '../components/artifacts/types';

interface Props {
  artifactContentQueryEnabled: boolean;
  isSummary?: boolean;
  artifact?: Artifact;
}

export function useFetchArtifactContentQuery({
  artifactContentQueryEnabled,
  isSummary,
  artifact,
}: Props) {
  return useQuery<FetchedArtifactContent, Error>({
    queryKey: [
      'artifactContentForSection',
      artifact?.name || 'no-artifact-selected',
      artifactContentQueryEnabled,
    ],
    queryFn: async () => {
      if (!artifact || !artifact.fetchUrl || isSummary) {
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
    enabled: artifactContentQueryEnabled,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  });
}
