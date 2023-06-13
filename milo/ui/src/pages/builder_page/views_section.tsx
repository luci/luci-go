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

import { CircularProgress, Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import {
  useAuthState,
  useGetAccessToken,
} from '@/common/components/auth_state_provider';
import { PrpcClientExt } from '@/common/libs/prpc_client_ext';
import { extractProject } from '@/common/libs/utils';
import { BuilderID } from '@/common/services/buildbucket';
import {
  MiloInternal,
  QueryConsolesRequest,
} from '@/common/services/milo_internal';

const PAGE_SIZE = 100;

function useConsoles(req: QueryConsolesRequest) {
  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useQuery({
    queryKey: [identity, MiloInternal.SERVICE, 'QueryConsoles', req],
    queryFn: async () => {
      const miloInternalService = new MiloInternal(
        new PrpcClientExt(
          { host: '', insecure: location.protocol === 'http:' },
          getAccessToken
        )
      );
      return await miloInternalService.queryConsoles(
        req,
        // Let react-query manage caching.
        { acceptCache: false, skipUpdate: true }
      );
    },
  });
}

export interface ViewsSectionProps {
  readonly builderId: BuilderID;
}

export function ViewsSection({ builderId }: ViewsSectionProps) {
  const { data, error, isError, isLoading } = useConsoles({
    predicate: {
      builder: builderId,
    },
    pageSize: PAGE_SIZE,
  });

  if (isError) {
    throw error;
  }

  return (
    <>
      <h3>Views</h3>
      {isLoading ? (
        <CircularProgress />
      ) : (
        <>
          <ul>
            {data.consoles?.map((c) => {
              const project = extractProject(c.realm);
              const consoleLabel = `${project} / ${c.id}`;
              return (
                <li key={consoleLabel}>
                  <Link href={`/p/${project}/g/${c.id}`}>{consoleLabel}</Link>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </>
  );
}
