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

import { MiloInternalClientImpl } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { BatchedMiloInternalClientImpl } from '@/proto_utils/batched_clients/milo_internal_client';

import { usePrpcServiceClient } from './prpc_query';

export function useMiloInternalClient() {
  return usePrpcServiceClient({
    host: SETTINGS.milo.host,
    ClientImpl: MiloInternalClientImpl,
  });
}

export function useBatchedMiloInternalClient() {
  return usePrpcServiceClient({
    host: SETTINGS.milo.host,
    ClientImpl: BatchedMiloInternalClientImpl,
  });
}
