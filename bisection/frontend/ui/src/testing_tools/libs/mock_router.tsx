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

import { FC, ReactElement } from 'react';
import { QueryClient, QueryClientProvider } from 'react-query';
import { BrowserRouter as Router, Route, Routes } from 'react-router-dom';

import { render } from '@testing-library/react';

interface Props {
  children?: ReactElement;
}

/**
 * Renders a component with a mock router and a mock query client.
 *
 * @param ui The UI component to render.
 * @param route The route that the current component is at, defaults to '/'.
 * @param routeDefinition The definition of the current route,
 *                        useful for getting route params.
 * @return The render result.
 */
export const renderWithRouterAndClient = (
  ui: React.ReactElement,
  route = '/',
  routeDefinition = ''
) => {
  const wrapper: FC = ({ children }: Props) => {
    return (
      <Router>
        <Routes>
          <Route
            path={routeDefinition ? routeDefinition : route}
            element={children}
          />
        </Routes>
      </Router>
    );
  };
  window.history.pushState({}, 'Test page', route);
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
    {
      wrapper,
    }
  );
};
