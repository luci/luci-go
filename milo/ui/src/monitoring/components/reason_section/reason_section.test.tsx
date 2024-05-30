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

import { configuredTrees } from '@/monitoring/util/config';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ReasonSection } from './reason_section';

it('displays message when no test results', async () => {
  render(
    <FakeContextProvider>
      <ReasonSection
        failureBuildUrl="https://ci.chromium.org/p/chromium/b/1234"
        tree={configuredTrees[0]}
        reason={{
          num_failing_tests: 0,
          step: 'test step',
          tests: [],
        }}
      />
    </FakeContextProvider>,
  );
  expect(
    screen.getByText('No test result data available.'),
  ).toBeInTheDocument();
});

it('displays test info', async () => {
  render(
    <FakeContextProvider>
      <ReasonSection
        failureBuildUrl="https://ci.chromium.org/p/chromium/b/1234"
        tree={configuredTrees[0]}
        reason={{
          num_failing_tests: 0,
          step: 'test step',
          tests: [
            {
              test_id: 'ninja://test.Example',
              realm: 'ci',
              test_name: 'test.Example',
              cluster_name: 'chromium/rules-v2/4242',
              variant_hash: '1234',
              cur_counts: {
                unexpected_results: 10,
                total_results: 10,
              },
              cur_start_hour: '9:00',
              regression_start_position: 123450,
              prev_counts: {
                unexpected_results: 1,
                total_results: 100,
              },
              prev_end_hour: '8:00',
              regression_end_position: 123456,
              ref_hash: '5678',
            },
          ],
        }}
      />
    </FakeContextProvider>,
  );
  expect(screen.getByText('test.Example')).toBeInTheDocument();
  expect(screen.getByText('0% Passing (0/10)')).toBeInTheDocument();
  expect(screen.getByText('6 commits')).toBeInTheDocument();
  expect(screen.getByText('99% Passing (99/100)')).toBeInTheDocument();
});
