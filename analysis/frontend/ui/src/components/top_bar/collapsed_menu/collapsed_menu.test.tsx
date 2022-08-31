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

import { Home } from '@mui/icons-material';
import {
  fireEvent,
  screen,
} from '@testing-library/react';

import { renderWithRouter } from '../../../testing_tools/libs/mock_router';
import { AppBarPage } from '../top_bar';
import { TopBarContextProvider } from '../top_bar_context';
import CollapsedMenu from './collapsed_menu';

describe('test CollapsedMenu component', () => {
  const pages: AppBarPage[] = [
    {
      title: 'Clusters',
      url: '/Clusters',
      icon: Home,
    },
  ];

  it('given a set of pages, then should display them in a menu', async () => {
    renderWithRouter(
        <CollapsedMenu pages={pages}/>,
    );

    await screen.findByText('LUCI Analysis');

    expect(screen.getByText('Clusters')).toBeInTheDocument();
  });

  it('when clicking on menu button then the menu should be visible', async () => {
    renderWithRouter(
        <TopBarContextProvider >
          <CollapsedMenu pages={pages}/>
        </TopBarContextProvider>,
    );

    await screen.findByText('LUCI Analysis');

    expect(screen.getByTestId('collapsed-menu')).not.toBeVisible();

    await fireEvent.click(screen.getByTestId('collapsed-menu-button'));

    expect(screen.getByTestId('collapsed-menu')).toBeVisible();
  });
});
