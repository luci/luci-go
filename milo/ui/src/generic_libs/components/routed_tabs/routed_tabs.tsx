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

import { Tabs, TabsProps } from '@mui/material';
import { useReducer } from 'react';
import { Outlet } from 'react-router-dom';

import {
  ActiveTabContextProvider,
  ActiveTabUpdaterContextProvider,
} from './context';
import { reducer } from './reducer';

/**
 * A tabs implementation based on MUI tabs and react router.
 *
 * It's similar to MUI `<Tabs />` except that
 *  * `value` on `<RoutedTabs />` is managed automatically, and
 *  * its children must be `<RoutedTab />` instead of `<Tab />` from MUI, and
 *  * the tab content components must be mounted as child routes and should
 *    define its associated tab ID with `useDeclareTabId('tab-id')`, and
 *  * it contains an `<Outlet />`.
 */
export function RoutedTabs(props: Omit<TabsProps, 'value'>) {
  const [state, dispatch] = useReducer(reducer, { activeTab: null });

  return (
    <ActiveTabUpdaterContextProvider value={dispatch}>
      <ActiveTabContextProvider
        value={{ activeTabId: state.activeTab?.tabId ?? null }}
      >
        <Tabs {...props} value={state.activeTab?.tabId ?? false} />
        <Outlet />
      </ActiveTabContextProvider>
    </ActiveTabUpdaterContextProvider>
  );
}
