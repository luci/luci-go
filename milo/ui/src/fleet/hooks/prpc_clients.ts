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

import { TokenType } from '@/common/components/auth_state_provider';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { FleetConsoleClientImpl } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FleetClientImpl } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/rpc/fleet.pb';

export function useFleetConsoleClient() {
  return usePrpcServiceClient({
    host: SETTINGS.fleetConsole.host,
    tokenType: TokenType.Id,
    insecure: SETTINGS.fleetConsole.host.startsWith('localhost'), // use http for local development
    ClientImpl: FleetConsoleClientImpl,
  });
}

export function useUfsClient() {
  return usePrpcServiceClient({
    host: SETTINGS.ufs.host,
    insecure: SETTINGS.fleetConsole.host.startsWith('localhost'), // use http for local development
    ClientImpl: FleetClientImpl,
    // Namespace is needed to call UFS.
    additionalHeaders: { namespace: 'os' },
  });
}
