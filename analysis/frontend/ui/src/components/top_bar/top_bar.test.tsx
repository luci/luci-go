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
import '@testing-library/jest-dom';

import { screen } from '@testing-library/react';

import {
  renderWithRouter,
  renderWithRouterAndClient,
} from '../../testing_tools/libs/mock_router';
import TopBar from './top_bar';

describe('test TopBar component', () => {
  beforeAll(() => {
    window.email = 'test@google.com';
    window.avatar = '/example.png';
    window.fullName = 'Test Name';
    window.logoutUrl = '/logout';
  });

  it('should render logo and user email', async () => {
    renderWithRouter(
        <TopBar />,
    );

    await screen.findAllByText('LUCI Analysis');

    expect(screen.getByText(window.email)).toBeInTheDocument();
  });

  it('given a route with a project then should display pages', async () => {
    renderWithRouterAndClient(
        <TopBar />,
        '/p/chrome',
        '/p/:project',
    );

    await screen.findAllByText('LUCI Analysis');

    expect(screen.getAllByText('Clusters')).toHaveLength(2);
    expect(screen.getAllByText('Rules')).toHaveLength(2);
  });
});
