// Copyright 2022 The LUCI Authors.
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

import { applySnapshot } from 'mobx-state-tree';
import React, { createContext, ReactNode, useContext } from 'react';

import { AppState } from '../context/app_state';
import { Store, StoreEnv, StoreInstance, StoreSnapshotIn } from '../context/store';

let clientStore: StoreInstance;
export const StoreContext = createContext<StoreInstance | null>(null);

export function useStore() {
  const context = useContext(StoreContext);
  if (!context) {
    throw new Error('useStore must be used within StoreProvider');
  }

  return context;
}

export interface StoreProviderProps {
  appState: AppState;
  children: ReactNode;
  snapshot?: StoreSnapshotIn;
}

export function StoreProvider({ appState, children, snapshot }: StoreProviderProps) {
  const store = initializeStore({ appState }, snapshot);
  return <StoreContext.Provider value={store}>{children}</StoreContext.Provider>;
}

export const DEFAULT_SNAPSHOT: StoreSnapshotIn = Object.freeze({ searchPage: { searchQuery: '' } });

export function initializeStore(storeEnv: StoreEnv, snapshot = DEFAULT_SNAPSHOT) {
  if (!clientStore) {
    clientStore = Store.create(snapshot, storeEnv);
    return clientStore;
  }

  applySnapshot(clientStore, snapshot);
  return clientStore;
}
