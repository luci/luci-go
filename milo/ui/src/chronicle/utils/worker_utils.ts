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

let chronicleWorkerPolicy: TrustedTypePolicy | undefined;

/**
 * Web Workers require a sanitized script URL via the Trusted Types API
 * in order to protect against things like XSS.
 */
export function getTrustedWorkerURL(
  url: string,
): TrustedScriptURL | URL | string {
  if (typeof window === 'undefined') return url;

  const tt = window.trustedTypes;
  if (!tt) return url;

  if (!chronicleWorkerPolicy) {
    try {
      chronicleWorkerPolicy = tt.createPolicy('chronicle-graph-worker', {
        createScriptURL: (u: string) => u,
      }) as TrustedTypePolicy;
    } catch {
      return url;
    }
  }

  if (chronicleWorkerPolicy) {
    return chronicleWorkerPolicy.createScriptURL(url.toString());
  }

  return url;
}
