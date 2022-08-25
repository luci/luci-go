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
  render,
  screen,
} from '@testing-library/react';

import UserActions from './user_actions';

describe('test UserActions component', () => {
  beforeAll(() => {
    window.email = 'test@google.com';
    window.avatar = '/example.png';
    window.fullName = 'Test Name';
    window.logoutUrl = '/logout';
  });

  it('should display user email and logout url', async () => {
    render(
        <UserActions />,
    );

    await screen.getByText(window.email);

    expect(screen.getByRole('img')).toHaveAttribute('src', window.avatar);
    expect(screen.getByRole('img')).toHaveAttribute('alt', window.fullName);
    expect(screen.getByTestId('useractions_logout')).toHaveAttribute('href', window.logoutUrl);
  });

  it('when clicking on email button then should display logout url', async () => {
    render(
        <UserActions />,
    );

    await screen.getByText(window.email);

    expect(screen.getByTestId('user-settings-menu')).not.toBeVisible();

    await fireEvent.click(screen.getByText(window.email));

    expect(screen.getByTestId('user-settings-menu')).toBeVisible();
  });
});
