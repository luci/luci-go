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

export enum BuildPageTab {
  Overview = 'overview',
  TestResults = 'test-results',
  Infra = 'infra',
  RelatedBuilds = 'related-builds',
  Timeline = 'timeline',
  Blamelist = 'blamelist',
}

/**
 * The *default* "default tab" of the build page.
 *
 * The default tab of the build page can be customized by the user. This value
 * is the value of the default tab if it has not been customized by the user.
 */
export const INITIAL_DEFAULT_TAB = BuildPageTab.Overview;

/**
 * Parse the tab string into the `BuildPageTab` enum. If the tab string is
 * invalid, return null.
 */
export function parseTab(tab: string): BuildPageTab | null {
  if (Object.values(BuildPageTab).includes(tab as BuildPageTab)) {
    return tab as BuildPageTab;
  }
  return null;
}
