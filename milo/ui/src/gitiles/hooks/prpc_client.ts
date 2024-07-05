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

import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { FusedGitilesClientImpl } from '@/gitiles/api/fused_gitiles_client';
import { CrrevClientImpl } from '@/proto/infra/appengine/cr-rev/frontend/api/v1/service.pb';

export function useCrRevClient() {
  return usePrpcServiceClient({
    host: SETTINGS.crRev.host,
    ClientImpl: CrrevClientImpl,
  });
}

export function useGitilesClient(host: string) {
  return usePrpcServiceClient(
    {
      host,
      ClientImpl: FusedGitilesClientImpl,
    },
    { crRevHost: SETTINGS.crRev.host },
  );
}
