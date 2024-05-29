// Copyright 2023 The LUCI Authors.
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

import { cleanup, render, screen } from '@testing-library/react';
import fetchMockJest from 'fetch-mock-jest';
import { act } from 'react';
import { ErrorBoundary } from 'react-error-boundary';

import {
  ClusterRequest,
  ClusterResponse,
  ClustersClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { ResultDBClientImpl } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  TestResult,
  TestStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { mockFetchTextArtifact } from '@/test_verdict/components/artifact_tags/text_artifact/testing_tools/text_artifact_mock';
import { resetSilence, silence } from '@/testing_tools/console_filter';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { NEVER_PROMISE } from '@/testing_tools/utils';

import { FakeTestVerdictContextProvider } from '../testing_tools/fake_context';

import { TestResults } from './test_results';

const SILENCED_ERROR_MAGIC_STRING = ' <d48dda8>';

describe('<TestResults />', () => {
  const resultName =
    'invocations/u-chrome-bot-2023-10-25-09-08-00-26592efa1f477db0/tests/' +
    'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y/results/87ecc8c3-00063';

  beforeEach(() => {
    jest.useFakeTimers();
    silence('error', (err) => `${err}`.includes(SILENCED_ERROR_MAGIC_STRING));
    silence('error', (err) =>
      `${err}`.includes(
        'React will try to recreate this component tree from scratch using the error boundary you provided',
      ),
    );
    mockFetchTextArtifact(
      resultName + '/artifacts/Test%20Log',
      'artifact content',
    );
    jest
      .spyOn(ResultDBClientImpl.prototype, 'ListArtifacts')
      .mockImplementation(async () => NEVER_PROMISE);
  });
  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    resetSilence();
    jest.resetAllMocks();
    fetchMockJest.reset();
  });

  it('given a successful luci analysis fetch, then should display similar failures', async () => {
    jest
      .spyOn(ClustersClientImpl.prototype, 'Cluster')
      .mockImplementation((_: ClusterRequest) => {
        return Promise.resolve(
          ClusterResponse.fromPartial({
            clusteredTestResults: Object.freeze([
              {
                clusters: Object.freeze([
                  {
                    clusterId: {
                      algorithm: 'reason-failure',
                      id: '12345abcd',
                    },
                  },
                ]),
              },
            ]),
            clusteringVersion: {
              algorithmsVersion: 1,
              configVersion: '1',
              rulesVersion: '1',
            },
          }),
        );
      });
    render(
      <FakeContextProvider>
        <FakeTestVerdictContextProvider>
          <TestResults
            results={[
              {
                result: TestResult.fromPartial({
                  testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
                  name: resultName,
                  resultId: '87ecc8c3-00063',
                  status: TestStatus.FAIL,
                  summaryHtml: '<text-artifact artifact-id="Test Log" />',
                  startTime: '2023-10-25T09:01:00.167244802Z',
                  duration: {
                    seconds: '55',
                    nanos: 567000000,
                  },
                  tags: Object.freeze([
                    {
                      key: 'ancestor_buildbucket_ids',
                      value: '8766287273535464561',
                    },
                    {
                      key: 'board',
                      value: 'betty-pi-arc',
                    },
                    {
                      key: 'bug_component',
                      value: 'b:95887',
                    },
                  ]),
                  failureReason: {
                    primaryErrorMessage:
                      'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
                      ' to validate field text on step 2: failed to validate input value: got: francais ;' +
                      ' want: français',
                  },
                }),
              },
            ]}
          />
        </FakeTestVerdictContextProvider>
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    await (() =>
      expect(screen.getByText('similar failures')).toBeInTheDocument());
  });

  it('given failed luci analysis fetch, the should still display details but show error', async () => {
    jest
      .spyOn(ClustersClientImpl.prototype, 'Cluster')
      .mockImplementation((_: ClusterRequest): Promise<ClusterResponse> => {
        return Promise.reject(
          new Error('fetch error' + SILENCED_ERROR_MAGIC_STRING),
        );
      });

    render(
      <FakeContextProvider>
        <ErrorBoundary fallback={<></>}>
          <FakeTestVerdictContextProvider>
            <TestResults
              results={[
                {
                  result: TestResult.fromPartial({
                    testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
                    name: resultName,
                    resultId: '87ecc8c3-00063',
                    status: TestStatus.FAIL,
                    summaryHtml: '<text-artifact artifact-id="Test Log" />',
                    startTime: '2023-10-25T09:01:00.167244802Z',
                    duration: {
                      seconds: '55',
                      nanos: 567000000,
                    },
                    tags: Object.freeze([
                      {
                        key: 'ancestor_buildbucket_ids',
                        value: '8766287273535464561',
                      },
                      {
                        key: 'board',
                        value: 'betty-pi-arc',
                      },
                      {
                        key: 'bug_component',
                        value: 'b:95887',
                      },
                    ]),
                    failureReason: {
                      primaryErrorMessage:
                        'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
                        ' to validate field text on step 2: failed to validate input value: got: francais ;' +
                        ' want: français',
                    },
                  }),
                },
              ]}
            />
          </FakeTestVerdictContextProvider>
        </ErrorBoundary>
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    await (() => {
      expect(screen.getByText('Details')).toBeInTheDocument();
      expect(screen.getByText('fetch error')).toBeInTheDocument();
      expect(screen.queryByText('similar failures')).not.toBeInTheDocument();
    });
  });
});
