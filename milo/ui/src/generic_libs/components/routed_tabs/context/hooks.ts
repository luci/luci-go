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

import { useContext, useEffect, useRef } from 'react';

import { ActiveTabContext, ActiveTabUpdaterContext } from './context';

/**
 * Get the tab ID of the active tab.
 */
export function useActiveTabId() {
  const ctx = useContext(ActiveTabContext);

  if (ctx === undefined) {
    throw new Error('useActiveTabId can only be used in a RoutedTabs');
  }

  return ctx.activeTabId;
}

/**
 * Declare the component with a tab ID. When the component is mounted, marked
 * the tab ID as activated.
 *
 * For each `<RoutedTabs />`, at most one tab can be activated at a time.
 */
export function useDeclareTabId(tabId: string) {
  const hookId = useRef();
  const dispatch = useContext(ActiveTabUpdaterContext);
  if (dispatch === undefined) {
    throw new Error('useDeclareTabId can only be used in a RoutedTabs');
  }

  useEffect(() => {
    dispatch({ type: 'activateTab', tabId, hookId });
  }, [dispatch, tabId]);

  // Wrap id in a ref so we don't need to declare it as a dependency.
  // We only need to deactivate tab when `dispatch` is changed (i.e. the
  // parent context is being switched), or when the component is being
  // unmounted.
  const hookIdRef = useRef(tabId);
  hookIdRef.current = tabId;
  useEffect(
    () => () =>
      dispatch({
        type: 'deactivateTab',
        tabId: hookIdRef.current,
        hookId: hookId,
      }),
    [dispatch],
  );
}
