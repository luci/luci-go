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

import TabContext from '@mui/lab/TabContext';

import { renderWithRouterAndClient } from './mock_router';

/**
 * Renders an MUI tab in a tab context, with mock router and query client.
 *
 * @param ui The UI component to render.
 * @param value The name of the tab to display.
 * @param route The route that the current component is at, defaults to '/'.
 * @param routeDefinition The definition of the current route,
 *                        useful for getting route params.
 * @return The render result.
 */
export const renderTabWithRouterAndClient = (
  ui: React.ReactElement,
  value = 'test',
  route = '/',
  routeDefinition = '',
) => {
  const tabContext = <TabContext value={value}>{ui}</TabContext>;
  return renderWithRouterAndClient(tabContext, route, routeDefinition);
};
