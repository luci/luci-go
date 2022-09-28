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

import {
  fireEvent,
  screen,
} from '@testing-library/react';

import { renderWithRouter } from '@/testing_tools/libs/mock_router';

import UserProfileButton from './user_profile_button';

describe('test UserActions component', () => {
  beforeAll(() => {
    window.logoutUrl = '/logout';
  });

  it('should display user email and logout url', async () => {
    window.email = 'test@google.com';
    window.avatar = '/example.png';
    window.fullName = 'Test Name';

    renderWithRouter(
        <UserProfileButton />,
        '/someurl',
    );

    await screen.getByText(window.email);

    expect(screen.getByRole('img')).toHaveAttribute('src', window.avatar);
    expect(screen.getByRole('img')).toHaveAttribute('alt', window.fullName);
    expect(screen.getByTestId('useractions_logout')).toHaveAttribute('href', '/logout?r=%2Fsomeurl');
  });

  it('clicking on email button then should display logout url', async () => {
    window.email = 'test@google.com';
    window.avatar = '/example.png';
    window.fullName = 'Test Name';

    renderWithRouter(
        <UserProfileButton />,
    );

    await screen.getByText(window.email);

    expect(screen.getByTestId('user-settings-menu')).not.toBeVisible();

    await fireEvent.click(screen.getByText(window.email));

    expect(screen.getByTestId('user-settings-menu')).toBeVisible();
  });
});
