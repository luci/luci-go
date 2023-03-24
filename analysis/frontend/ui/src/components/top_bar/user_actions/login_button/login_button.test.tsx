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
  screen,
} from '@testing-library/react';

import { renderWithRouter } from '@/testing_tools/libs/mock_router';

import LoginButton from './login_button';

describe('test UserActions component', () => {
  beforeAll(() => {
    window.loginUrl = '/login';
  });

  it('login in button should link to the login page', async () => {
    renderWithRouter(
        <LoginButton />,
        '/someurl',
    );

    screen.findByText('Log in');

    expect(screen.getByTestId('login_button')).toHaveAttribute('href', '/login?r=%2Fsomeurl');
  });
});
