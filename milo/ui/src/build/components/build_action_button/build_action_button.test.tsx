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

import { failedBuild, runningBuild } from '@/build/testing_tools/mock_builds';
import {
  BatchCheckPermissionsResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildActionButton } from './build_action_button';

describe('<BuildActionButton />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest
      .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
      .mockImplementation(async (req) =>
        BatchCheckPermissionsResponse.fromPartial({
          results: Object.fromEntries(req.permissions.map((p) => [p, true])),
        }),
      );
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('should show retry button when the build ended', async () => {
    render(
      <FakeContextProvider>
        <BuildActionButton build={failedBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('Retry Build');
  });

  it('should show cancel button when the build has not ended', async () => {
    render(
      <FakeContextProvider>
        <BuildActionButton build={runningBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('Cancel Build');
  });
});
