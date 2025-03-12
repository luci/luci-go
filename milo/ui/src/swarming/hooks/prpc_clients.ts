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
import {
  BotsClientImpl,
  TasksClientImpl,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

const ALLOWED_HOSTS = Object.freeze([
  SETTINGS.swarming.defaultHost,
  ...(SETTINGS.swarming.allowedHosts || []),
]);

export function useTasksClient(host: string) {
  if (!ALLOWED_HOSTS.includes(host)) {
    throw new Error(`'${host}' is not an allowed host`);
  }

  return usePrpcServiceClient({
    host,
    ClientImpl: TasksClientImpl,
  });
}

export function useBotsClient(host: string) {
  if (!ALLOWED_HOSTS.includes(host)) {
    throw new Error(`'${host}' is not an allowed host`);
  }

  return usePrpcServiceClient({
    host,
    ClientImpl: BotsClientImpl,
  });
}
