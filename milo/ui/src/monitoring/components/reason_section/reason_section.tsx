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

import BugReportIcon from '@mui/icons-material/BugReport';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';

import { DisableTestButton } from '@/monitoring/components/disable_button';
import {
  AlertReasonJson,
  AlertReasonTestJson,
  Bug,
  TreeJson,
} from '@/monitoring/util/server_json';

interface ReasonSectionProps {
  failureBBID: string;
  tree: TreeJson;
  reason: AlertReasonJson | null | undefined;
  bug?: Bug;
}

export const ReasonSection = (props: ReasonSectionProps) => {
  if (!props.reason?.tests?.length) {
    return <>No test result data available.</>;
  }
  return (
    <>
      <Table size="small" sx={{ border: 'solid 1px rgb(224, 224, 224)' }}>
        <TableHead>
          <TableRow>
            <TableCell>Failed Test</TableCell>
            <TableCell>Current Pass Rate</TableCell>
            <TableCell>Test Blamelist</TableCell>
            <TableCell>Previous Pass Rate</TableCell>
            <TableCell>Links</TableCell>
            <TableCell>Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {props.reason.tests?.map((t) => (
            <TestFailureRow
              key={t.test_id}
              test={t}
              tree={props.tree}
              bug={props.bug}
              failureBBID={props.failureBBID}
            />
          ))}
          {props.reason.tests.length < props.reason.num_failing_tests && (
            <TableRow>
              <TableCell colSpan={100}>
                {props.reason.num_failing_tests - props.reason.tests.length}{' '}
                more failing tests not shown
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </>
  );
};

const codeSearchLink = (t: AlertReasonTestJson): string => {
  let query = t.test_name;
  if (t.test_name.includes('#')) {
    // Guessing that it's a java test; the format expected is
    // test.package.TestClass#testMethod. For now, just split around the #
    const split = t.test_name.split('#');

    if (split.length == 2) {
      query = split[0] + ' function:' + split[1];
    }
  }
  return `https://cs.chromium.org/search/?q=${encodeURIComponent(query)}`;
};

const historyLink = (t: AlertReasonTestJson): string => {
  const project = encodeURIComponent(t.realm.split(':', 2)[0]);
  const testId = encodeURIComponent(t.test_id);
  const query = encodeURIComponent('VHASH:' + t.variant_hash);
  return `https://ci.chromium.org/ui/test/${project}/${testId}?q=${query}`;
};

interface SimilarFailuresLinkProps {
  test: AlertReasonTestJson;
}
const SimilarFailuresLink = ({ test }: SimilarFailuresLinkProps) => {
  const [project, algorithm, id] = test.cluster_name.split('/', 3);
  if (algorithm.startsWith('rules')) {
    return (
      <a
        style={{ display: 'inline-flex', alignItems: 'center' }}
        href={`https://luci-analysis.appspot.com/p/${project}/rules/${id}`}
        target="_blank"
        rel="noreferrer"
      >
        <BugReportIcon /> Bug
      </a>
    );
  }
  return (
    <a
      href={`https://luci-analysis.appspot.com/p/${project}/clusters/${algorithm}/${id}`}
      target="_blank"
      rel="noreferrer"
    >
      Similar Failures
    </a>
  );
};

interface TestFailureRowProps {
  test: AlertReasonTestJson;
  tree: TreeJson;
  bug?: Bug;
  failureBBID: string;
}

const TestFailureRow = ({
  test,
  tree,
  bug,
  failureBBID,
}: TestFailureRowProps) => {
  const currentRate =
    test.cur_counts.total_results == 0
      ? undefined
      : 1 - test.cur_counts.unexpected_results / test.cur_counts.total_results;
  const previousRate =
    test.prev_counts.total_results == 0
      ? undefined
      : 1 -
        test.prev_counts.unexpected_results / test.prev_counts.total_results;
  const cellColor = (rate: number | undefined) => {
    if (rate === undefined) {
      return undefined;
    }
    if (rate > 0.98) {
      return 'var(--success-bg-color)';
    }
    if (rate < 0.02) {
      return 'var(--failure-bg-color)';
    }
    return 'var(--started-bg-color)';
  };

  return (
    <TableRow hover>
      <TableCell>{test.test_name}</TableCell>
      <TableCell sx={{ backgroundColor: cellColor(currentRate) }}>
        {currentRate === undefined
          ? 'No longer running'
          : `${Math.round(currentRate * 100)}% Passing (${
              test.cur_counts.total_results - test.cur_counts.unexpected_results
            }/${test.cur_counts.total_results})`}
      </TableCell>
      <TableCell>
        {test.regression_start_position} - {test.regression_end_position}
      </TableCell>
      <TableCell sx={{ backgroundColor: cellColor(previousRate) }}>
        {previousRate === undefined
          ? 'New test'
          : `${Math.round(previousRate * 100)}% Passing (${
              test.prev_counts.total_results -
              test.prev_counts.unexpected_results
            }/${test.prev_counts.total_results})`}
      </TableCell>
      <TableCell>
        {isChromiumSrc(tree.name) ? (
          <a href={codeSearchLink(test)} target="_blank" rel="noreferrer">
            Code Search
          </a>
        ) : null}
        {' | '}
        <a href={historyLink(test)} target="_blank" rel="noreferrer">
          History
        </a>
        {' | '}
        <SimilarFailuresLink test={test} />
      </TableCell>
      <TableCell>
        {isChromiumSrc(tree.name) ? (
          <DisableTestButton
            bug={bug}
            testID={test.test_id}
            failureBBID={failureBBID}
          />
        ) : null}
      </TableCell>
    </TableRow>
  );
};

// chromium trees are trees that are monitoring the chromium/src repo.
// Code search and test disabling work only on chromium/src.
const chromiumSrcTrees = [
  'chromium',
  'chromium.gpu',
  'chromium.perf',
  'chrome_browser_release',
];
const isChromiumSrc = (tree: string): boolean =>
  chromiumSrcTrees.indexOf(tree) != -1;
