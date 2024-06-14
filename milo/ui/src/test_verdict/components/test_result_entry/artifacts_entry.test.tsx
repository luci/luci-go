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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { cleanup, render, screen } from '@testing-library/react';
import { userEvent } from '@testing-library/user-event';
import { act } from 'react';

import {
  ListArtifactsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactsEntry } from './artifacts_entry';

describe('<ArtifactsEntry />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('should render both test result level and invocation level artifacts', async () => {
    jest
      .spyOn(ResultDBClientImpl.prototype, 'ListArtifacts')
      .mockImplementation(async (req) => {
        if (req.parent === 'invocations/inv1') {
          return ListArtifactsResponse.fromPartial({
            artifacts: [
              {
                name: 'invocations/inv1/artifacts/inv-artifact1',
                artifactId: 'inv-artifact1',
              },
              {
                name: 'invocations/inv1/artifacts/inv-artifact2',
                artifactId: 'inv-artifact2',
              },
            ],
          });
        }
        if (req.parent === 'invocations/inv1/tests/test1/results/result1') {
          return ListArtifactsResponse.fromPartial({
            artifacts: [
              {
                name: 'invocations/inv1/tests/test1/results/result1/artifacts/result-artifact1',
                artifactId: 'result-artifact1',
              },
              {
                name: 'invocations/inv1/tests/test1/results/result1/artifacts/result-artifact2',
                artifactId: 'result-artifact2',
              },
            ],
          });
        }
        throw new Error('unexpected parent');
      });

    render(
      <FakeContextProvider>
        <ArtifactsEntry testResultName="invocations/inv1/tests/test1/results/result1" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    userEvent.click(screen.getByText('Artifacts:'));
    await act(() => jest.runAllTimersAsync());

    expect(screen.getByText('inv-artifact1')).toBeInTheDocument();
    expect(screen.getByText('inv-artifact2')).toBeInTheDocument();
    expect(screen.getByText('result-artifact1')).toBeInTheDocument();
    expect(screen.getByText('result-artifact2')).toBeInTheDocument();
  });

  it('should show warning when users have no access to the artifacts', async () => {
    jest
      .spyOn(ResultDBClientImpl.prototype, 'ListArtifacts')
      .mockRejectedValue(
        new GrpcError(RpcCode.PERMISSION_DENIED, 'no access to artifacts'),
      );

    render(
      <FakeContextProvider>
        <ArtifactsEntry testResultName="invocations/inv1/tests/test1/results/result1" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    userEvent.click(screen.getByText('Artifacts:'));
    await act(() => jest.runAllTimersAsync());

    expect(
      screen.getByText('Failed to query result artifacts'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Failed to query invocation artifacts'),
    ).toBeInTheDocument();
  });
});
