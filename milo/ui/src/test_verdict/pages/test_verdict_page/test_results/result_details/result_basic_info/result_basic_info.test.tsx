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

import { render, screen } from '@testing-library/react';
import { userEvent } from '@testing-library/user-event';

import {
  TestResult,
  TestStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { OutputClusterEntry } from '@/test_verdict/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FakeTestVerdictContextProvider } from '../../../testing_tools/fake_context';
import { TestResultsProvider } from '../../context';
import { ResultDataProvider } from '../context';

import { ResultBasicInfo } from './result_basic_info';

describe('<ResultBasicInfo />', () => {
  const failedResult = TestResult.fromPartial({
    testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
    name:
      'invocations/u-chrome-bot-2023-10-25-09-08-00-26592efa1f477db0/tests/' +
      'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y/results/87ecc8c3-00063',
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
        ' to validate field text on step 2: failed to validate input value: got: francais ; want: français',
    },
  });

  function renderBasicInfo(result: TestResult) {
    return render(
      <FakeContextProvider>
        <FakeTestVerdictContextProvider>
          <TestResultsProvider results={[]}>
            <ResultDataProvider result={result}>
              <ResultBasicInfo />
            </ResultDataProvider>
          </TestResultsProvider>
        </FakeTestVerdictContextProvider>
      </FakeContextProvider>,
    );
  }

  it('given error reason, should display error block', async () => {
    renderBasicInfo(failedResult);
    await screen.findByText('Details');
    expect(
      screen.getByText(
        'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
          ' to validate field text on step 2: failed to validate input value: got: francais ; want: français',
      ),
    ).toBeInTheDocument();
  });

  it('given error reason, should be displayed instead of `Details` when collapsed', async () => {
    renderBasicInfo(failedResult);

    await screen.findByText('Details');
    await userEvent.click(screen.getByText('Details'));

    expect(screen.queryByText('Details')).not.toBeInTheDocument();
    expect(
      screen.getAllByText(
        'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
          ' to validate field text on step 2: failed to validate input value: got: francais ; want: français',
      ),
    ).toHaveLength(2);
  });

  it('given a duration, then should be displayed and formatted correctly', async () => {
    renderBasicInfo(failedResult);

    await screen.findByText('Details');

    expect(screen.getByText('56s')).toBeInTheDocument();
  });

  it('given an invocation id that belongs to a task, then should display swarming task link', async () => {
    // set up
    const failedResult = TestResult.fromPartial({
      testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
      name:
        'invocations/task-chromium-swarm.appspot.com-659c82e40f213711/tests/' +
        'ninja:%2F%2F:blink_web_tests%2Ffast%2Fbackgrounds%2Fbackground-position-parsing.html/results/b5b8a970-03989',
      resultId: '87ecc8c3-00063',
      status: TestStatus.FAIL,
      summaryHtml: '<text-artifact artifact-id="Test Log" />',
      startTime: '2023-10-25T09:01:00.167244802Z',
      failureReason: {
        primaryErrorMessage:
          'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
          ' to validate field text on step 2: failed to validate input value: got: francais ; want: français',
      },
    });

    // act
    renderBasicInfo(failedResult);

    // verify
    expect(screen.getByText('659c82e40f213711')).toBeInTheDocument();
  });

  it('given a reason cluster, then should display similar failures link', async () => {
    // set up
    const failedResult = TestResult.fromPartial({
      testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
      name:
        'invocations/task-chromium-swarm.appspot.com-659c82e40f213711/tests/' +
        'ninja:%2F%2F:blink_web_tests%2Ffast%2Fbackgrounds%2Fbackground-position-parsing.html/results/b5b8a970-03989',
      resultId: '87ecc8c3-00063',
      status: TestStatus.FAIL,
      summaryHtml: '<text-artifact artifact-id="Test Log" />',
      startTime: '2023-10-25T09:01:00.167244802Z',
      failureReason: {
        primaryErrorMessage:
          'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
          ' to validate field text on step 2: failed to validate input value: got: francais ; want: français',
      },
    });
    const clustersMap: Map<string, readonly OutputClusterEntry[]> = new Map();
    clustersMap.set(failedResult.resultId, [
      {
        clusterId: {
          algorithm: 'reason-failure-reason',
          id: '123456abcd',
        },
        bug: undefined,
      },
    ]);
    // act
    render(
      <FakeContextProvider
        pageMeta={{
          project: 'chromium',
        }}
      >
        <FakeTestVerdictContextProvider>
          <TestResultsProvider results={[]} clustersMap={clustersMap}>
            <ResultDataProvider result={failedResult}>
              <ResultBasicInfo />
            </ResultDataProvider>
          </TestResultsProvider>
        </FakeTestVerdictContextProvider>
      </FakeContextProvider>,
    );

    // verify
    await screen.findByText('Details');
    await (() =>
      expect(screen.getByText('similar failures')).toBeInTheDocument());
  });

  it('given a list of clusters with bugs, should display a list of bugs', async () => {
    // set up
    const failedResult = TestResult.fromPartial({
      testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
      name:
        'invocations/task-chromium-swarm.appspot.com-659c82e40f213711/tests/' +
        'ninja:%2F%2F:blink_web_tests%2Ffast%2Fbackgrounds%2Fbackground-position-parsing.html/results/b5b8a970-03989',
      resultId: '87ecc8c3-00063',
      status: TestStatus.FAIL,
      summaryHtml: '<text-artifact artifact-id="Test Log" />',
      startTime: '2023-10-25T09:01:00.167244802Z',
      failureReason: {
        primaryErrorMessage:
          'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
          ' to validate field text on step 2: failed to validate input value: got: francais ; want: français',
      },
    });
    const clustersMap: Map<string, readonly OutputClusterEntry[]> = new Map();
    clustersMap.set(failedResult.resultId, [
      {
        clusterId: {
          algorithm: 'reason-failure-reason',
          id: '123456abcd',
        },
        bug: {
          id: '123456',
          linkText: 'b/123456',
          system: 'buganizer',
          url: 'http://buganizer.example/123456',
        },
      },
      {
        clusterId: {
          algorithm: 'test-failure-reason',
          id: '123456abcdf',
        },
        bug: {
          id: '1234567',
          linkText: 'b/1234567',
          system: 'buganizer',
          url: 'http://buganizer.example/1234567',
        },
      },
    ]);

    // act
    render(
      <FakeContextProvider
        pageMeta={{
          project: 'chromium',
        }}
      >
        <FakeTestVerdictContextProvider>
          <TestResultsProvider results={[]} clustersMap={clustersMap}>
            <ResultDataProvider result={failedResult}>
              <ResultBasicInfo />
            </ResultDataProvider>
          </TestResultsProvider>
        </FakeTestVerdictContextProvider>
      </FakeContextProvider>,
    );

    // verify
    await screen.findByText('Details');
    await (() =>
      expect(screen.getByText('similar failures')).toBeInTheDocument());
    expect(screen.getByText('Related bugs:')).toBeInTheDocument();
    expect(screen.getByText('b/123456')).toBeInTheDocument();
    expect(screen.getByText('b/1234567')).toBeInTheDocument();
  });
});
