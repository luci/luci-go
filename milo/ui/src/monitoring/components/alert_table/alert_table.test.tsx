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

import { act, fireEvent, render, screen } from '@testing-library/react';

import { configuredTrees } from '@/monitoring/util/config';
import { AlertJson, RevisionJson } from '@/monitoring/util/server_json';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AlertTable } from './alert_table';

it('displays an alert', async () => {
  render(
    <FakeContextProvider>
      <AlertTable tree={configuredTrees[0]} alerts={[alert]} bugs={[]} />
    </FakeContextProvider>,
  );
  expect(screen.getByText('linux-rel')).toBeInTheDocument();
  expect(screen.getByText('compile')).toBeInTheDocument();
});

it('expands an alert on click', async () => {
  render(
    <FakeContextProvider>
      <AlertTable tree={configuredTrees[0]} alerts={[alert]} bugs={[]} />
    </FakeContextProvider>,
  );
  expect(screen.getByText('compile')).toBeInTheDocument();
  expect(screen.queryByText('test.Example')).toBeNull();
  await act(() => fireEvent.click(screen.getByText('compile')));
  expect(screen.getByText('test.Example')).toBeInTheDocument();
});

it('sorts alerts on header click', async () => {
  render(
    <FakeContextProvider>
      <AlertTable
        tree={configuredTrees[0]}
        alerts={[alert, alert2]}
        bugs={[]}
      />
    </FakeContextProvider>,
  );
  expect(screen.getByText('linux-rel')).toBeInTheDocument();
  expect(screen.getByText('win-rel')).toBeInTheDocument();
  // Sort asc
  await act(() => fireEvent.click(screen.getByText('Failed Builder')));
  let linux = screen.getByText('linux-rel');
  let win = screen.getByText('win-rel');
  expect(linux.compareDocumentPosition(win)).toBe(
    Node.DOCUMENT_POSITION_FOLLOWING,
  );
  // Sort desc
  await act(() => fireEvent.click(screen.getByText('Failed Builder')));
  linux = screen.getByText('linux-rel');
  win = screen.getByText('win-rel');
  expect(linux.compareDocumentPosition(win)).toBe(
    Node.DOCUMENT_POSITION_PRECEDING,
  );
});

const revision: RevisionJson = {
  author: 'Player 1',
  branch: 'main',
  commit_position: 123,
  description: 'A fun CL.',
  git_hash: '12345677',
  host: 'host',
  link: 'host/123',
  repo: 'chromium/src',
  when: 1,
};

const alert: AlertJson = {
  key: 'alert-1',
  title: 'Step "compile" failuing on builder linux-rel',
  body: '',
  links: null,
  resolved: false,
  severity: 1,
  start_time: new Date().valueOf(),
  tags: null,
  time: new Date().valueOf(),
  type: 'compile',
  extension: {
    builders: [
      {
        project: 'chromium',
        builder_group: 'linux',
        bucket: 'ci',
        name: 'linux-rel',
        build_status: 'FAILED',
        count: 2,
        start_time: new Date().valueOf(),
        url: '/p/chromium/b/linux-rel',
        failing_tests_trunc: '',
        latest_passing: 3,
        first_failing_rev: { ...revision, commit_position: 10 },
        first_failure: 23,
        first_failure_build_number: 342,
        first_failure_url: '/b/342',
        last_passing_rev: { ...revision, commit_position: 5 },
        latest_failure: 46,
        latest_failure_build_number: 356,
        latest_failure_url: '/b/356',
      },
    ],
    culprits: null,
    has_findings: false,
    is_finished: false,
    is_supported: false,
    regression_ranges: [],
    suspected_cls: null,
    tree_closer: false,
    reason: {
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
    },
  },
  bug: '0',
  silenceUntil: '0',
};

const alert2: AlertJson = {
  key: 'alert-2',
  title: 'Step "compile" failuing on builder win-rel',
  body: '',
  links: null,
  resolved: false,
  severity: 1,
  start_time: new Date().valueOf(),
  tags: null,
  time: new Date().valueOf(),
  type: 'compile',
  extension: {
    builders: [
      {
        project: 'chromium',
        builder_group: 'windows',
        bucket: 'ci',
        name: 'win-rel',
        build_status: 'FAILED',
        count: 2,
        start_time: new Date().valueOf(),
        url: '/p/chromium/b/win-rel',
        failing_tests_trunc: '',
        latest_passing: 3,
        first_failing_rev: { ...revision, commit_position: 10 },
        first_failure: 23,
        first_failure_build_number: 342,
        first_failure_url: '/b/342',
        last_passing_rev: { ...revision, commit_position: 5 },
        latest_failure: 46,
        latest_failure_build_number: 356,
        latest_failure_url: '/b/356',
      },
    ],
    culprits: null,
    has_findings: false,
    is_finished: false,
    is_supported: false,
    regression_ranges: [],
    suspected_cls: null,
    tree_closer: false,
    reason: {
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
    },
  },
  bug: '0',
  silenceUntil: '0',
};
