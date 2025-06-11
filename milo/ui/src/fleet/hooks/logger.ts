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

import { useMutation } from '@tanstack/react-query';

import { LogFrontendRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { useFleetConsoleClient } from './prpc_clients';

/**
 * Returns a function which sends logs to the backend
 * In local environment does nothing
 */
export function useLogger() {
  const client = useFleetConsoleClient();
  // It actually doesn't mutate anything.
  const logFrontendMutation = useMutation({
    mutationFn: (request: LogFrontendRequest) => {
      if (isRunningOnLocalhost()) {
        // Dry run in case the frontend is run in local
        return Promise.resolve({});
      }
      return client.LogFrontend(request);
    },
  });

  return { logger: logFrontendMutation.mutate };
}

function isRunningOnLocalhost() {
  const hostname = window.location.hostname;

  return hostname === 'localhost' || hostname === '127.0.0.1';
}
