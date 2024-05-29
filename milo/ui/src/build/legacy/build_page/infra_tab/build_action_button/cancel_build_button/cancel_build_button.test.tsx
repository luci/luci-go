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

import { render, screen } from '@testing-library/react';
import { act } from 'react';

import { PERM_BUILDS_CANCEL } from '@/build/constants';
import {
  runningBuild,
  scheduledToBeCanceledBuild,
} from '@/build/testing_tools/mock_builds';
import {
  BatchCheckPermissionsResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { CancelBuildButton } from './cancel_build_button';

describe('<CancelBuildButton />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('should disable when the user has no permission to cancel build', async () => {
    render(
      <FakeContextProvider>
        <CancelBuildButton build={runningBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
  });

  it('should enable when the user has the permission to cancel build', async () => {
    const batchCheckPermissionsMock = jest
      .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
      .mockImplementation(async () =>
        BatchCheckPermissionsResponse.fromPartial({
          results: { [PERM_BUILDS_CANCEL]: true },
        }),
      );

    render(
      <FakeContextProvider>
        <CancelBuildButton build={runningBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    expect(batchCheckPermissionsMock).toHaveBeenCalledWith(
      expect.objectContaining({
        realm: `${runningBuild.builder.project}:${runningBuild.builder.bucket}`,
        permissions: [PERM_BUILDS_CANCEL],
      }),
    );

    const button = screen.getByRole('button');
    expect(button).not.toBeDisabled();
  });

  it('should disable when the build is already scheduled to be canceled', async () => {
    jest
      .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
      .mockImplementation(async () =>
        BatchCheckPermissionsResponse.fromPartial({
          results: { [PERM_BUILDS_CANCEL]: true },
        }),
      );

    render(
      <FakeContextProvider>
        <CancelBuildButton build={scheduledToBeCanceledBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
  });
});
