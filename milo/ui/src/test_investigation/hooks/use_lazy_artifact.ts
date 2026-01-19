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

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { GetArtifactRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

export function useLazyArtifact(artifact?: Artifact) {
  const client = useResultDbClient();

  return useQuery({
    ...client.GetArtifact.query(
      GetArtifactRequest.fromPartial({
        name: artifact?.name || '',
      }),
    ),
    enabled: !!artifact?.name && !artifact.fetchUrl,
    staleTime: Infinity,
    select: (data) => {
      // If we already have the artifact (from props) and it has fetchUrl,
      // we might not even run this query due to enabled check.
      // But if we do run it, we return the fresh data.
      return data;
    },
    initialData: artifact?.fetchUrl ? artifact : undefined,
    initialDataUpdatedAt: artifact?.fetchUrl ? Date.now() : undefined,
  });
}
