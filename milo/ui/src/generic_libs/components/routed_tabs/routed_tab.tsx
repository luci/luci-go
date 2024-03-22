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

import { Tab, TabProps } from '@mui/material';
import { Link } from 'react-router-dom';

import { useActiveTabId } from './context';

export interface RoutedTabProps
  extends Omit<TabProps<typeof Link>, 'component' | 'value'> {
  // This prop has to have name `value` because mui <Tabs /> iterate over its
  // children and collect the `value` props from them to build a list of valid
  // tabs.
  readonly value: string;
  /**
   * Hide the tab if the tab is not the current active one.
   *
   * Sometimes it is useful to conditionally display a tab selector (e.g. base
   * on the user permission, whether the data is available, etc). But this can
   * get confusing when users hit a tab without a tab selector via a URL route.
   * Use this flag to hide the tab selector only when the tab is inactive.
   */
  readonly hideWhenInactive?: boolean;
}

/**
 * A tab implementation based on MUI tab and react router.
 *
 * It's similar to MUI `<Tab />` except that
 *  * `value` on `<RoutedTabs />` is managed automatically, and
 *  * it contains `<Outlet />`.
 */
export function RoutedTab({
  value,
  hideWhenInactive = false,
  ...props
}: RoutedTabProps) {
  const activeTabId = useActiveTabId();
  const shouldHide = hideWhenInactive && value !== activeTabId;

  // Do not render anything when the tab is hidden.
  // Using `display: none` may cause errors if the tab has tooltip in the label.
  if (shouldHide) {
    return <></>;
  }

  return <Tab {...props} value={value} component={Link} />;
}
