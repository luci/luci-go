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

import { usePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { QueryTestMetadataRequest, ResultDb } from '@/common/services/resultdb';

const MAIN_GIT_REF = 'refs/heads/main';

// TODO: query with pagination.
export function useTestMetadata(request: QueryTestMetadataRequest) {
  return usePrpcQuery({
    host: SETTINGS.resultdb.host,
    Service: ResultDb,
    method: 'queryTestMetadata',
    request,
    options: {
      select: (res) => {
        if (!res.testMetadata?.length) {
          return null;
        }
        // Select the main branch. Fallback to the first element if main branch
        // is not found.
        return (
          res.testMetadata.find(
            (m) => m.sourceRef.gitiles?.ref === MAIN_GIT_REF,
          ) || res.testMetadata[0]
        );
      },
    },
  });
}
