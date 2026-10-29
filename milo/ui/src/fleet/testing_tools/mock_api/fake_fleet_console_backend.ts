// Copyright 2026 The LUCI Authors.
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

import {
  CheckPermissionRequest,
  CheckPermissionResponse,
  FleetConsole,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { createIrmFake } from './fake_irm';
import {
  Aip160Matcher,
  createPriorityRulesFake,
  rpc,
  RpcJsonHandler,
} from './fake_priority_rules';
import { createRepairQueueFake } from './fake_repair_queue';
import { FleetConsoleMockAPI } from './mock_api_handler';

export { createIrmFake, createPriorityRulesFake, createRepairQueueFake };
export type { Aip160Matcher, RpcJsonHandler };

const DEFAULT_AUTH_STATE = {
  identity: 'user:user@example.com',
  email: 'user@example.com',
  picture: '',
  accessToken: 'mock-access-token',
  idToken: 'mock-id-token',
  accessTokenExpiry: 9999999999,
  idTokenExpiry: 9999999999,
};

export function createPermissionsFake() {
  let defaultHasPermission = true;
  const groupOverrides = new Map<string, boolean>();

  const service: Pick<FleetConsole, 'CheckPermission'> = {
    async CheckPermission(req: CheckPermissionRequest) {
      return CheckPermissionResponse.create({
        hasPermission: groupOverrides.get(req.group) ?? defaultHasPermission,
      });
    },
  };

  return {
    ...service,
    handlers: {
      CheckPermission: rpc(
        CheckPermissionRequest,
        CheckPermissionResponse,
        service.CheckPermission,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    setDefaultPermission(val: boolean) {
      defaultHasPermission = val;
    },
    setGroupPermission(group: string, val: boolean) {
      groupOverrides.set(group, val);
    },
    reset() {
      defaultHasPermission = true;
      groupOverrides.clear();
    },
  };
}

function prpcResponse(
  payload: unknown,
  grpcCode = 0,
  status = grpcCode === 0 ? 200 : 500,
) {
  return new Response(")]}'\n" + JSON.stringify(payload), {
    status,
    headers: {
      'Content-Type': 'application/json',
      'X-Prpc-Grpc-Code': String(grpcCode),
    },
  });
}

export function createFakeFleetConsoleBackend(
  matchesFilter: Aip160Matcher = (item, expr) =>
    FleetConsoleMockAPI.matchesAip160Filter(item, expr),
) {
  let authState = { ...DEFAULT_AUTH_STATE };
  let nativeFetch: typeof globalThis.fetch | null = null;

  const priorityRules = createPriorityRulesFake(matchesFilter);
  const repairQueue = createRepairQueueFake({
    priorityRules,
    matchesFilter,
    getCurrentUserEmail: () => authState.email || 'user@example.com',
  });
  const irm = createIrmFake();
  const permissions = createPermissionsFake();

  const staticResponses = new Map<string, unknown>();
  const errors = new Map<string, { grpcCode: number; message: string }>();
  const deferred = new Map<string, Promise<unknown>>();
  const calls = new Map<
    string,
    { url: string; payload: Record<string, unknown> }[]
  >();

  const endpoints: Record<string, RpcJsonHandler> = {
    ...priorityRules.handlers,
    ...repairQueue.handlers,
    ...irm.handlers,
    ...permissions.handlers,
  };

  const reset = () => {
    authState = { ...DEFAULT_AUTH_STATE };
    priorityRules.seed([]);
    repairQueue.seed({
      repairQueueItems: [],
      totalSize: 0,
      nextPageToken: '',
    });
    irm.seed([]);
    permissions.reset();
    staticResponses.clear();
    errors.clear();
    deferred.clear();
    calls.clear();
  };

  reset();

  return {
    priorityRules,
    repairQueue,
    irm,
    permissions,
    setAuthState(override: Partial<typeof DEFAULT_AUTH_STATE>) {
      authState = { ...authState, ...override };
    },
    setStaticResponse(method: string, response: unknown) {
      staticResponses.set(method, response);
    },
    setError(method: string, grpcCode: number, message: string) {
      errors.set(method, { grpcCode, message });
    },
    clearError(method: string) {
      errors.delete(method);
    },
    setDeferredResponse(method: string, promise: Promise<unknown>) {
      deferred.set(method, promise);
    },
    getCalls(method: string) {
      return calls.get(method) ?? [];
    },
    clearCalls() {
      calls.clear();
    },
    reset,
    install() {
      if (nativeFetch) return;
      nativeFetch = globalThis.fetch;
      globalThis.fetch = async (
        input: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url;

        if (url.includes('/auth/openid/state')) {
          return new Response(JSON.stringify(authState), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }

        const prpcMatch = url.match(/\/prpc\/[^/]+\/([^/?#]+)/);
        if (!prpcMatch) {
          return nativeFetch!.call(globalThis, input, init);
        }

        const method = prpcMatch[1];
        let payload: Record<string, unknown> = {};
        if (typeof init?.body === 'string' && init.body) {
          try {
            payload = JSON.parse(init.body);
          } catch {
            // Ignore non-JSON body parse errors
          }
        }

        const history = calls.get(method) ?? [];
        history.push({ url, payload });
        calls.set(method, history);

        const pending = deferred.get(method);
        if (pending) {
          deferred.delete(method);
          const resolved = (await pending) as
            | { __isError?: boolean; grpcCode?: number; message?: string }
            | undefined;
          if (resolved?.__isError) {
            return prpcResponse(
              { message: resolved.message ?? 'pRPC Server Error' },
              resolved.grpcCode ?? 13,
            );
          }
          return prpcResponse(resolved ?? {});
        }

        const err = errors.get(method);
        if (err) {
          return prpcResponse({ message: err.message }, err.grpcCode);
        }

        if (endpoints[method]) {
          const response = await endpoints[method](payload);
          return prpcResponse(response);
        }

        if (staticResponses.has(method)) {
          return prpcResponse(staticResponses.get(method));
        }

        return prpcResponse({});
      };
    },
    uninstall() {
      if (!nativeFetch) return;
      globalThis.fetch = nativeFetch;
      nativeFetch = null;
    },
  };
}

export const fakeFleetConsoleBackend = createFakeFleetConsoleBackend();
