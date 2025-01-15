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
import { SourceIndexClientImpl } from '@/proto/go.chromium.org/luci/source_index/proto/v1/source_index.pb';

export function useSourceIndexClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciSourceIndex.host,
    ClientImpl: SourceIndexClientImpl,
  });
}

/**
 * A list of gitiles host that are known to allow cross-origin requests from
 * LUCI UI.
 *
 * `FusedGitilesClientImpl` sends requests to the gitiles host directly if
 * LUCI UI is allowed to send cross-origin requests to them. Otherwise, the
 * requests are sent through `MiloInternal`, which adds 200~600ms overhead.
 *
 * Follow the instruction at go/luci-ui-allow-cross-origin-requests-to-gitiles
 * to allow LUCI UI to send cross-origin requests to more gitiles hosts.
 */
const CORS_ENABLED_GITILES_HOSTS = Object.freeze([
  'chromium.googlesource.com',
  'chrome-internal.googlesource.com',
  'fuchsia.googlesource.com',
  'turquoise-internal.googlesource.com',
  'webrtc.googlesource.com',
]);

export function useGitilesClient(host: string) {
  return usePrpcServiceClient(
    {
      host,
      ClientImpl: FusedGitilesClientImpl,
    },
    {
      sourceIndexHost: SETTINGS.luciSourceIndex.host,
      useMiloGitilesProxy:
        self.location.hostname === 'localhost' ||
        !CORS_ENABLED_GITILES_HOSTS.includes(host),
    },
  );
}
