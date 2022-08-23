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
import { createContext, ReactNode, useContext, useEffect, useRef } from 'react';

import { AppState } from '../context/app_state';
import { Store, StoreInstance, StoreSnapshotIn } from '../store';

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
  snapshot?: StoreSnapshotIn;
  children: ReactNode;
}

export function StoreProvider({ appState, snapshot, children }: StoreProviderProps) {
  const store = useRef<StoreInstance | null>(null);
  if (!store.current) {
    store.current = Store.create(snapshot);
    store.current.setAppState(appState);
  }

  // Applying a snapshot can cause components to update if its already being
  // observed. Therefore we need to wrap `applySnapshot` in `useEffect`.
  useEffect(() => {
    // Add this check to help TS infer the type. This should never happen
    // because we've initialized the store when first creating the component.
    if (!store.current) {
      throw new Error('unreachable');
    }

    if (snapshot) {
      applySnapshot(store.current, snapshot);
    }
    store.current?.setAppState(appState);
  }, [snapshot, appState]);

  return <StoreContext.Provider value={store.current}>{children}</StoreContext.Provider>;
}
