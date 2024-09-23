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

import { ReactNode, useMemo } from 'react';

import {
  TrackedSearchParamKeysProvider,
  useTrackedSearchParamKeys,
} from './context';

export interface TrackSearchParamKeysProps {
  /**
   * The search param keys to track.
   *
   * A descendant page view tracker will collect all search param keys tracked
   * by its ancestors. Specifying tracking keys in the parent route helps
   * avoiding repeating those tracking keys in all the child routes.
   *
   * Updates to untracked search param keys will not trigger a new page view
   * event. This is designed to prevent the tracker from generating excessive
   * amount of page view events when frequently updated state (e.g. filters,
   * toggles) are stored in the search param.
   *
   * Untracked search param keys will not be sent to GA. This is designed to
   * reduce the chance of recording PII in GA. Recording PII in GA is forbidden
   * by our policy. See http://go/ooga-config.
   */
  readonly keys: readonly string[];
  readonly children: ReactNode;
}

/**
 * Defines the list of search param keys to track in a route component.
 */
export function TrackSearchParamKeys({
  keys,
  children,
}: TrackSearchParamKeysProps) {
  const parentKeys = useTrackedSearchParamKeys();

  // We don't expect there to be a lot of tracked keys. Use a simple
  // implementation to filter out duplicated keys and sort them.
  const newKeys = new Set([...parentKeys, ...keys]);
  const sortedNewKeys = [...newKeys].sort();

  // Use a stable reference to prevent cascading update when a parent component
  // is rerendered.
  const stableNewKeys = useMemo(
    () => sortedNewKeys,
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [JSON.stringify(sortedNewKeys)],
  );

  return (
    <TrackedSearchParamKeysProvider value={stableNewKeys}>
      {children}
    </TrackedSearchParamKeysProvider>
  );
}
