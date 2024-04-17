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

import { act, render, screen } from '@testing-library/react';

import { PERM_BUILDS_ADD } from '@/build/constants';
import {
  failedBuild,
  unretriableBuild,
} from '@/build/testing_tools/mock_builds';
import {
  BatchCheckPermissionsResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RetryBuildButton } from './retry_build_button';

describe('<RetryBuildButton />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('should disable when the user has no permission to schedule build', async () => {
    render(
      <FakeContextProvider>
        <RetryBuildButton build={failedBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
  });

  it('should not disable when the user has the permission to schedule build', async () => {
    const batchCheckPermissionsMock = jest
      .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
      .mockImplementation(async () =>
        BatchCheckPermissionsResponse.fromPartial({
          results: { [PERM_BUILDS_ADD]: true },
        }),
      );

    render(
      <FakeContextProvider>
        <RetryBuildButton build={failedBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    expect(batchCheckPermissionsMock).toHaveBeenCalledWith(
      expect.objectContaining({
        realm: `${failedBuild.builder.project}:${failedBuild.builder.bucket}`,
        permissions: expect.arrayContaining([PERM_BUILDS_ADD]),
      }),
    );

    const button = screen.getByRole('button');
    expect(button).not.toBeDisabled();
  });

  it('should disable when the build cannot be retried', async () => {
    jest
      .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
      .mockImplementation(async () =>
        BatchCheckPermissionsResponse.fromPartial({
          results: { [PERM_BUILDS_ADD]: true },
        }),
      );

    render(
      <FakeContextProvider>
        <RetryBuildButton build={unretriableBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
  });
});
