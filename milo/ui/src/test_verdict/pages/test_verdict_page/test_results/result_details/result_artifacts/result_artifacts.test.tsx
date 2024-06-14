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
import { userEvent } from '@testing-library/user-event';
import { act } from 'react';

import {
  ListArtifactsRequest,
  ListArtifactsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { createFakeTestResult } from '../../testing_tools/utils';
import { ResultDataProvider } from '../context';

import { ResultArtifacts } from './result_artifacts';

describe('<ResultArtifacts />', () => {
  const resultName =
    'invocations/u-chrome-bot-2023-10-25-09-08-00-26592efa1f477db0/tests/' +
    'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y/results/87ecc8c3-00063';

  beforeEach(() => {
    jest
      .spyOn(ResultDBClientImpl.prototype, 'ListArtifacts')
      .mockImplementation((req: ListArtifactsRequest) => {
        if (!req.parent.includes('results')) {
          return Promise.resolve(
            ListArtifactsResponse.fromPartial({
              artifacts: [
                {
                  artifactId: 'inv_log1.txt',
                },
                {
                  artifactId: 'inv_log2.txt',
                },
                {
                  artifactId: 'inv_log3.txt',
                },
              ],
              nextPageToken: '',
            }),
          );
        } else {
          return Promise.resolve(
            ListArtifactsResponse.fromPartial({
              artifacts: [
                {
                  artifactId: 'result_log.txt',
                },
              ],
              nextPageToken: '',
            }),
          );
        }
      });
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.resetAllMocks();
  });

  it('given valid artifact lists, then should display their links', async () => {
    render(
      <FakeContextProvider>
        <ResultDataProvider result={createFakeTestResult(resultName)}>
          <ResultArtifacts />
        </ResultDataProvider>
      </FakeContextProvider>,
    );
    await screen.findByText('Result artifacts 1');
    expect(screen.getByText('Invocation artifacts 3')).toBeInTheDocument();
    expect(screen.getByText('result_log.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log1.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log2.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log3.txt')).toBeInTheDocument();
  });

  it('using search input must filter artifacts', async () => {
    // Setup
    const user = userEvent.setup({
      advanceTimers: jest.advanceTimersByTime,
    });

    render(
      <FakeContextProvider>
        <ResultDataProvider result={createFakeTestResult(resultName)}>
          <ResultArtifacts />
        </ResultDataProvider>
      </FakeContextProvider>,
    );
    await screen.findByText('Invocation artifacts 3');
    expect(screen.getByText('inv_log1.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log1.txt')).not.toBeVisible();
    expect(screen.getByText('inv_log2.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log2.txt')).not.toBeVisible();
    expect(screen.getByText('inv_log3.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log3.txt')).not.toBeVisible();

    // Expand
    await user.click(screen.getByText('Invocation artifacts 3'));
    expect(screen.getByText('inv_log1.txt')).toBeVisible();
    expect(screen.getByText('inv_log2.txt')).toBeVisible();
    expect(screen.getByText('inv_log3.txt')).toBeVisible();
    expect(screen.getByLabelText('Search invocation artifacts')).toBeVisible();

    // Search
    await act(async () => {
      await user.click(screen.getByLabelText('Search invocation artifacts'));
      await user.keyboard('inv_log1');
      jest.runAllTimers();
    });

    // Verify
    expect(screen.getByText('inv_log1.txt')).toBeInTheDocument();
    expect(screen.queryByText('inv_log2.txt')).not.toBeInTheDocument();
    expect(screen.queryByText('inv_log3.txt')).not.toBeInTheDocument();
  });
});
