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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';

import { renderWithRouter } from '@/testing_tools/libs/mock_router';

import LoadErrorAlert from './load_error_alert';

describe('Test ErrorAlert component', () => {
  it('given a permission denied error and no login, it should show a login link.', async () => {
    window.isAnonymous = true;
    window.loginUrl = '/login';

    const err = new GrpcError(RpcCode.PERMISSION_DENIED, 'caller does not have permission a.b.c');
    renderWithRouter(
        <LoadErrorAlert
          entityName="SomeEntity"
          error={err}
        />,
        '/someurl',
    );

    await screen.findByText('Failed to load SomeEntity');
    expect(screen.getByText('Failed to load SomeEntity')).toBeInTheDocument();
    expect(screen.getByTestId('error_login_link')).toHaveAttribute('href', '/login?r=%2Fsomeurl');
  });

  it('given another error, it should should show the error description', async () => {
    window.isAnonymous = true;
    window.loginUrl = '/login';

    const err = new GrpcError(RpcCode.INVALID_ARGUMENT, 'failure_filter: invalid syntax');
    renderWithRouter(<LoadErrorAlert
      entityName="SomeEntity"
      error={err}
    />);

    await screen.findByText('Failed to load SomeEntity');
    expect(screen.getByText('Failed to load SomeEntity')).toBeInTheDocument();
    expect(screen.getByText('INVALID_ARGUMENT: failure_filter: invalid syntax')).toBeInTheDocument();
  });
});
