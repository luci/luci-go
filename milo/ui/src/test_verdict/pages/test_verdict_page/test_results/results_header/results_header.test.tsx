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

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { OutputTestResultBundle } from '@/test_verdict/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestResultsProvider } from '../context';

import { ResultsHeader } from './results_header';

const sampleResults = [
  TestResultBundle.fromPartial({
    result: {
      testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
      name:
        'invocations/u-chrome-bot-2023-10-25-09-08-00-26592efa1f477db0/tests/' +
        'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y/results/87ecc8c3-00063',
      resultId: '87ecc8c3-000623',
      status: TestStatus.FAIL,
      summaryHtml: '<text-artifact artifact-id="Test Log" />',
      startTime: '2023-10-25T09:01:00.167244802Z',
      duration: {
        seconds: '55',
        nanos: 567000000,
      },
      tags: [
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
      ],
      failureReason: {
        primaryErrorMessage:
          'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
          ' to validate field text on step 2: failed to validate input value: got: francais ;' +
          ' want: français',
      },
    },
  }),
  TestResultBundle.fromPartial({
    result: {
      testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
      name:
        'invocations/u-chrome-bot-2023-10-25-09-08-00-26592efa1f477db0/tests/' +
        'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y/results/87ecc8c3-00063',
      resultId: '87ecc8c3-00063',
      status: TestStatus.PASS,
      summaryHtml: '<text-artifact artifact-id="Test Log" />',
      startTime: '2023-10-25T09:01:00.167244802Z',
      duration: {
        seconds: '55',
        nanos: 567000000,
      },
      tags: [
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
      ],
    },
  }),
] as readonly OutputTestResultBundle[];

describe('<ResultsHeader />', () => {
  it('given no selected result in the route, then should select the first failed result', async () => {
    render(
      <FakeContextProvider>
        <TestResultsProvider results={sampleResults}>
          <ResultsHeader />
        </TestResultsProvider>
      </FakeContextProvider>,
    );
    await screen.findByText('Result 1');

    expect(screen.getByText('Result 1')).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('given a selected result in the route, then should select that result', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?resultIndex=1'],
        }}
      >
        <TestResultsProvider results={sampleResults}>
          <ResultsHeader />
        </TestResultsProvider>
      </FakeContextProvider>,
    );
    await screen.findByText('Result 1');

    expect(screen.getByText('Result 2')).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });
});
