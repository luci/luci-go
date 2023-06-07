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

import {
  useAuthState,
  useGetAccessToken,
} from '@/common/components/auth_state_provider';
import { PrpcClientExt } from '@/common/libs/prpc_client_ext';
import {
  QueryTestMetadataRequest,
  ResultDb,
  TestMetadataDetail,
} from '@/common/services/resultdb';

const MAIN_GIT_REF = 'refs/heads/main';

// TODO: query with pagination.
export function useTestMetadata(req: QueryTestMetadataRequest) {
  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useQuery({
    queryKey: [identity, ResultDb.SERVICE, 'QueryTestMetadata', req],
    queryFn: async () => {
      const resultDBService = new ResultDb(
        new PrpcClientExt({ host: CONFIGS.RESULT_DB.HOST }, getAccessToken)
      );
      const res = await resultDBService.queryTestMetadata(req, {
        acceptCache: false,
        skipUpdate: true,
      });
      if (!res.testMetadata?.length) {
        return {} as TestMetadataDetail;
      }
      // Select the main branch. Fallback to the first element if main branch not found.
      return (
        res.testMetadata.find(
          (m) => m.sourceRef.gitiles?.ref === MAIN_GIT_REF
        ) || res.testMetadata[0]
      );
    },
  });
}
