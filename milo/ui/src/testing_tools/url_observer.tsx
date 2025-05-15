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

import { useLocation } from 'react-router';

export const locationCallback = jest.fn((_path: string) => {});

export interface Location {
  readonly pathname: string;
  readonly search: string;
  readonly searchParams: { [key: string]: string };
  readonly hash: string;
}

export interface URLObserverProps {
  /**
   * Called when the component is rerendered. Useful to get the location in a
   * structured format. When you pass in a `const mockedCallback = jest.fn()`,
   * the location can be tested with
   * ```
   * expect(urlCallback).toHaveBeenLastCalledWith(
   *   expect.objectContaining({
   *     pathname: ...,
   *     search: {
   *       key1: ...,
   *       ...,
   *     },
   *     hash: ...,
   *   }),
   * );
   * ```
   */
  readonly callback?: (location: Location) => void;
}

/**
 * A simple component that renders the URL to a span with data-testid="url".
 * This is useful for testing against URLs.
 */
export function URLObserver({ callback = () => {} }: URLObserverProps) {
  const location = useLocation();
  callback({
    pathname: location.pathname,
    hash: location.hash,
    search: location.search,
    searchParams: Object.fromEntries(
      new URLSearchParams(location.search).entries(),
    ),
  });
  return (
    <span data-testid="url">
      {location.pathname + location.search + location.hash}
    </span>
  );
}
