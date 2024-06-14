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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';

import {
  runningBuild,
  scheduledToBeCanceledBuild,
} from '@/build/testing_tools/mock_builds';
import {
  BuildsClientImpl,
  CancelBuildRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { CancelBuildDialog } from './cancel_build_dialog';

describe('<CancelBuildDialog />', () => {
  let cancelBuildStub: jest.SpiedFunction<BuildsClientImpl['CancelBuild']>;
  beforeEach(() => {
    jest.useFakeTimers();
    cancelBuildStub = jest
      .spyOn(BuildsClientImpl.prototype, 'CancelBuild')
      .mockImplementation(async () => scheduledToBeCanceledBuild);
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('should not trigger cancel request when reason is not provided', async () => {
    const onCloseSpy = jest.fn();

    render(
      <FakeContextProvider>
        <CancelBuildDialog
          buildId={runningBuild.id}
          open
          onClose={onCloseSpy}
        />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByText('Confirm'));
    await act(() => jest.runOnlyPendingTimersAsync());
    expect(onCloseSpy).not.toHaveBeenCalled();
    expect(cancelBuildStub).not.toHaveBeenCalled();
    expect(screen.getByText('Reason is required')).toBeInTheDocument();
  });

  it('should trigger cancel request when reason is provided', async () => {
    const onCloseSpy = jest.fn();

    render(
      <FakeContextProvider>
        <CancelBuildDialog
          buildId={runningBuild.id}
          open
          onClose={onCloseSpy}
        />
      </FakeContextProvider>,
    );

    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'need to stop build' },
    });
    fireEvent.click(screen.getByText('Confirm'));
    await act(() => jest.runOnlyPendingTimersAsync());
    expect(onCloseSpy).toHaveBeenCalledTimes(1);
    expect(screen.queryByText('Reason is required')).not.toBeInTheDocument();
    expect(cancelBuildStub).toHaveBeenCalledTimes(1);
    expect(cancelBuildStub).toHaveBeenLastCalledWith(
      CancelBuildRequest.fromPartial({
        id: runningBuild.id,
        summaryMarkdown: 'need to stop build',
      }),
    );
  });
});
