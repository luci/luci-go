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

import { useMatches, UIMatch } from 'react-router';

/**
 * Hook to extract the `appId` from the current route matches.
 */
export function useActiveAppId(): string | undefined {
  try {
    const matches = useMatches() as UIMatch<unknown, { appId?: string }>[];
    const appIdMatches = matches.filter((m) => m.handle?.appId !== undefined);
    return appIdMatches.length > 0
      ? appIdMatches[appIdMatches.length - 1].handle?.appId
      : undefined;
  } catch (_e) {
    // Return undefined if used outside a router context (e.g. in isolated unit tests).
    return undefined;
  }
}
