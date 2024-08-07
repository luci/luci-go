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
import { VirtuosoMockContext } from 'react-virtuoso';

import {
  ListArtifactsRequest,
  ListArtifactsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { createFakeTestResult } from '../../testing_tools/utils';
import { ResultDataProvider } from '../provider';

import { ArtifactsTree } from './artifacts_tree';
import { ResultLogsProvider } from './context';

describe('<ArtifactsTree />', () => {
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
                  name: 'inv/inv_log1.txt',
                },
                {
                  artifactId: 'inv_log2.txt',
                  name: 'inv/inv_log2.txt',
                },
                {
                  artifactId: 'inv_log3.txt',
                  name: 'inv/inv_log3.txt',
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
                  name: 'result_log.txt',
                  sizeBytes: '3000',
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
  it('given a list of artifacts then should display a tree of them', async () => {
    render(
      <FakeContextProvider>
        <ResultDataProvider result={createFakeTestResult(resultName)}>
          <ResultLogsProvider>
            <VirtuosoMockContext.Provider
              value={{ viewportHeight: 1000, itemHeight: 100 }}
            >
              <ArtifactsTree />
            </VirtuosoMockContext.Provider>
          </ResultLogsProvider>
        </ResultDataProvider>
      </FakeContextProvider>,
    );
    await screen.findByText('Result artifacts');
    expect(screen.getByText('Invocation artifacts')).toBeInTheDocument();
    expect(screen.getByText('result_log.txt')).toBeInTheDocument();
    expect(screen.getByText('[3 kB]')).toBeInTheDocument();
    expect(screen.getByText('inv_log1.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log2.txt')).toBeInTheDocument();
    expect(screen.getByText('inv_log3.txt')).toBeInTheDocument();
  });

  it('given a selected then should display it as selected', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [
            '/test-verdict?selectedArtifact=result_log.txt&selectedArtifactSource=result',
          ],
        }}
        mountedPath="test-verdict"
      >
        <ResultDataProvider result={createFakeTestResult(resultName)}>
          <ResultLogsProvider>
            <VirtuosoMockContext.Provider
              value={{ viewportHeight: 1000, itemHeight: 100 }}
            >
              <ArtifactsTree />
            </VirtuosoMockContext.Provider>
          </ResultLogsProvider>
        </ResultDataProvider>
      </FakeContextProvider>,
    );
    await screen.findByText('Selected artifact:');
    expect(screen.getAllByText('result_log.txt')).toHaveLength(2);
  });
});
