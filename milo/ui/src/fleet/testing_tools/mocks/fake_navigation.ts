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

import { createElement, useEffect } from 'react';
import { RouteObject, useLocation } from 'react-router';

interface LocationObserverProps {
  readonly onLocationChange: (pathname: string, search: string) => void;
}

const LocationObserver = ({ onLocationChange }: LocationObserverProps) => {
  const location = useLocation();
  useEffect(() => {
    onLocationChange(location.pathname, location.search);
  }, [location.pathname, location.search, onLocationChange]);
  return null;
};

/**
 * Observes real `react-router` navigation (both programmatic `useNavigate()`
 * calls and `<Link>` clicks) via `FakeContextProvider` sibling routes without
 * module-level `jest.mock('react-router')` side effects.
 */
export const fakeNavigation = () => {
  const state = {
    lastPath: undefined as string | undefined,
    lastSearch: undefined as string | undefined,
  };

  const handleLocationChange = (pathname: string, search: string) => {
    state.lastPath = pathname;
    state.lastSearch = search;
  };

  const observerElement = createElement(LocationObserver, {
    onLocationChange: handleLocationChange,
  });

  const siblingRoutes: RouteObject[] = [
    {
      path: '*',
      element: observerElement,
    },
  ];

  return {
    get lastPath(): string | undefined {
      return state.lastPath;
    },
    get lastSearch(): string | undefined {
      return state.lastSearch;
    },
    observerElement,
    siblingRoutes,
    clear: () => {
      state.lastPath = undefined;
      state.lastSearch = undefined;
    },
  };
};
