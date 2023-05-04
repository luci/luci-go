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

import { useQuery } from '@tanstack/react-query';

import { useAuthState, useGetAccessToken } from '../../components/auth_state_provider';
import { PrpcClientExt } from '../../libs/prpc_client_ext';
import { BuildsService, SearchBuildsRequest } from '../../services/buildbucket';

export function useBuilds(req: SearchBuildsRequest, opts = { keepPreviousData: false }) {
  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useQuery({
    queryKey: [identity, BuildsService.SERVICE, 'SearchBuilds', req],
    queryFn: async () => {
      const buildersService = new BuildsService(new PrpcClientExt({ host: CONFIGS.BUILDBUCKET.HOST }, getAccessToken));
      return buildersService.searchBuilds(req);
    },
    keepPreviousData: opts.keepPreviousData,
  });
}
