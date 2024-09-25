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
  Link,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';
import { Stack } from '@mui/material';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { DisableTestButton } from '@/monitoringv2/components/disable_button';
import {
  AlertBuilderJson,
  AlertReasonJson,
  AlertReasonTestJson,
  Bug,
  TreeJson,
} from '@/monitoringv2/util/server_json';
import { getBlamelistUrl } from '@/test_verdict/tools/url_utils';

import { PrefillFilterIcon } from '../alert_table/prefill_filter_icon';
import { TestHistorySparkline } from '../test_history_sparkline';

import { AIAnalysis } from './ai_analysis';

interface ReasonSectionProps {
  builder: AlertBuilderJson;
  failureBuildUrl: string;
  tree: TreeJson;
  reason?: AlertReasonJson;
  bug?: Bug;
}

export const ReasonSection = ({
  builder,
  tree,
  failureBuildUrl,
  reason,
  bug,
}: ReasonSectionProps) => {
  const [searchParams] = useSyncedSearchParams();
  const useAIAnalysis = searchParams.get('aia');

  if (!reason?.tests?.length) {
    return <>No test result data available.</>;
  }

  return (
    <>
      <Table size="small" sx={{ border: 'solid 1px rgb(224, 224, 224)' }}>
        <TableHead>
          <TableRow>
            <TableCell>Failed Test</TableCell>
            <TableCell width={125}>Test History</TableCell>
            <TableCell width={96}>Test Blamelist</TableCell>
            <TableCell width={250}>Links</TableCell>
            <TableCell width={80}>Actions</TableCell>
            {useAIAnalysis && <TableCell width={80}></TableCell>}
          </TableRow>
        </TableHead>
        <TableBody>
          {reason.tests.map((t) => (
            <TestFailureRow
              builder={builder}
              key={t.test_id}
              test={t}
              tree={tree}
              bug={bug}
              failureBuildUrl={failureBuildUrl}
            />
          ))}
          {reason.tests.length < reason.num_failing_tests && (
            <TableRow>
              <TableCell colSpan={100}>
                {reason.num_failing_tests - reason.tests.length} more failing
                tests not shown
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

    if (split.length === 2) {
      query = split[0] + ' function:' + split[1];
    }
  }
  return `https://cs.chromium.org/search/?q=${encodeURIComponent(query)}`;
};

interface SimilarFailuresLinkProps {
  test: AlertReasonTestJson;
}
const SimilarFailuresLink = ({ test }: SimilarFailuresLinkProps) => {
  const [project, algorithm, id] = test.cluster_name.split('/', 3);
  if (algorithm.startsWith('rules')) {
    return (
      <a
        css={{ display: 'inline-flex', alignItems: 'center' }}
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

const testBisectionLink = (t: AlertReasonTestJson): string => {
  if (!t.luci_bisection_result) {
    return '';
  }
  const project = encodeURIComponent(t.realm.split(':', 2)[0]);
  const analysisID = t.luci_bisection_result?.analysis_id;
  return `/ui/p/${project}/bisection/test-analysis/b/${analysisID}`;
};

interface TestFailureRowProps {
  builder: AlertBuilderJson;
  test: AlertReasonTestJson;
  tree: TreeJson;
  bug?: Bug;
  failureBuildUrl: string;
}

const TestFailureRow = ({
  builder,
  test,
  tree,
  bug,
  failureBuildUrl,
}: TestFailureRowProps) => {
  const [searchParams] = useSyncedSearchParams();
  const useAIAnalysis = searchParams.get('aia');

  const failureBuildId = /[0-9]+$/.exec(failureBuildUrl)?.[0];
  const blamelistUrl = getBlamelistUrl(
    {
      project: builder.project,
      refHash: test.ref_hash,
      testId: test.test_id,
      variantHash: test.variant_hash,
    },
    test.regression_end_position === test.regression_start_position
      ? undefined
      : test.regression_end_position.toString(),
  );
  return (
    <TableRow hover>
      <TableCell>
        <Stack direction="row">
          <Link
            href={`${failureBuildUrl}/test-results?q=ID%3A${test.test_id}`}
            target="_blank"
            rel="noreferrer"
          >
            {test.test_name}
          </Link>
          <PrefillFilterIcon filter={test.test_name} />
        </Stack>
      </TableCell>
      <TableCell>
        <TestHistorySparkline
          project={builder.project}
          subRealm={builder.bucket}
          testId={test.test_id}
          variantHash={test.variant_hash}
        />
      </TableCell>
      <TableCell>
        {test.regression_end_position === test.regression_start_position ? (
          'Unknown'
        ) : (
          <Link href={blamelistUrl} target="_blank" rel="noreferrer">
            {test.regression_end_position - test.regression_start_position}{' '}
            commits
          </Link>
        )}
      </TableCell>
      <TableCell>
        {isChromiumSrc(tree.name) ? (
          <Link href={codeSearchLink(test)} target="_blank" rel="noreferrer">
            Code Search
          </Link>
        ) : null}
        {testBisectionLink(test) && (
          <>
            {' | '}
            <Link
              href={testBisectionLink(test)}
              target="_blank"
              rel="noreferrer"
            >
              bisection
            </Link>
          </>
        )}
        {' | '}
        <SimilarFailuresLink test={test} />
      </TableCell>
      <TableCell>
        {isChromiumSrc(tree.name) && failureBuildId ? (
          <DisableTestButton
            bug={bug}
            testID={test.test_id}
            failureBBID={failureBuildId}
          />
        ) : null}
      </TableCell>
      {useAIAnalysis && (
        <TableCell>
          <AIAnalysis project={tree.project} test={test} />
        </TableCell>
      )}
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
  chromiumSrcTrees.indexOf(tree) !== -1;
