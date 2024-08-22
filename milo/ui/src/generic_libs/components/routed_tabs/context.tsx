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

import { Dispatch, createContext } from 'react';

import { Action } from './reducer';

export interface ActiveTabContextValue {
  readonly activeTabId: string | null;
}

export const ActiveTabContext = createContext<ActiveTabContextValue | null>(
  null,
);

export const ActiveTabContextProvider = ActiveTabContext.Provider;

// Keep the dispatch in a separate context so updating the active tab doesn't
// trigger refresh on components that only consume the dispatch action (which
// is rarely updated if at all).
export const ActiveTabUpdaterContext = createContext<Dispatch<Action> | null>(
  null,
);

export const ActiveTabUpdaterContextProvider = ActiveTabUpdaterContext.Provider;
