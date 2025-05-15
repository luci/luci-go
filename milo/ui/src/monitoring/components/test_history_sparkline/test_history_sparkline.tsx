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

import { LinearProgress } from '@mui/material';
import Link from '@mui/material/Link';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import {
  useTestHistoryClient,
  useResultDbClient,
} from '@/common/hooks/prpc_clients';
import { QueryTestHistoryRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import {
  TestVerdict,
  TestVerdictStatus,
  testVerdictStatusToJSON,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { BatchGetTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

const VERDICT_STATUS_CLASS_MAP: { [status: string]: string } = {
  [TestVerdictStatus.UNEXPECTED]: 'failure',
  [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]: 'failure',
  [TestVerdictStatus.FLAKY]: 'started',
  [TestVerdictStatus.EXONERATED]: 'canceled',
  [TestVerdictStatus.EXPECTED]: 'success',
};

export interface TestHistorySparklineProps {
  project: string;
  subRealm: string;
  testId: string;
  variantHash: string;
}

export const TestHistorySparkline = ({
  project,
  subRealm,
  testId,
  variantHash,
}: TestHistorySparklineProps) => {
  const client = useTestHistoryClient();
  const req = QueryTestHistoryRequest.fromPartial({
    predicate: {
      subRealm,
      variantPredicate: {
        hashEquals: variantHash,
      },
    },
    project,
    pageSize: 10,
    testId,
  });
  const { data, isError, error } = useQuery({
    ...client.Query.query(req),
  });
  if (isError) {
    return (
      <HtmlTooltip
        title={
          <>
            <div css={{ color: 'var(--failure-color)' }}>
              Error loading test history
            </div>
            <div>{(error as Error).message}</div>
          </>
        }
      >
        <div css={{ color: 'var(--failure-color)' }}>error</div>
      </HtmlTooltip>
    );
  }

  return (
    <div css={{ display: 'flex' }}>
      {data?.verdicts.map((verdict) => (
        <HtmlTooltip
          key={verdict.invocationId}
          title={
            <>
              <div>
                <span
                  css={{ fontWeight: '700' }}
                  className={VERDICT_STATUS_CLASS_MAP[verdict.status] || ''}
                >
                  {testVerdictStatusToJSON(verdict.status)}
                </span>{' '}
                {verdict.partitionTime ? (
                  <RelativeTimestamp
                    timestamp={DateTime.fromISO(verdict.partitionTime)}
                  />
                ) : null}
              </div>
              <div>
                {verdict.status !== TestVerdictStatus.EXPECTED ? (
                  <FailureReasonDisplay
                    invocationId={verdict.invocationId}
                    testId={verdict.testId}
                    variantHash={verdict.variantHash}
                  />
                ) : null}
              </div>
            </>
          }
        >
          <Link
            href={testVerdictLink(verdict)}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            <div
              className={`${VERDICT_STATUS_CLASS_MAP[verdict.status] || 'scheduled'}-bg-pattern`}
              css={{
                width: '11px',
                height: '18px',
              }}
            ></div>
          </Link>
        </HtmlTooltip>
      ))}
      <HtmlTooltip title="View full test history">
        <Link
          href={`/ui/test/${project}/${encodeURIComponent(testId)}?q=VHash%3A${variantHash}`}
          target="_blank"
          rel="noreferrer"
          onClick={(e) => e.stopPropagation()}
          css={{ textDecoration: 'none', color: '#000' }}
        >
          <div
            className={`scheduled-bg`}
            css={{
              width: '27px',
              height: '18px',
              fontSize: '24px',
              lineHeight: '12px',
              textAlign: 'center',
            }}
          >
            ...
          </div>
        </Link>
      </HtmlTooltip>
    </div>
  );
};

const testVerdictLink = (verdict: TestVerdict): string => {
  if (verdict.invocationId.startsWith('build-')) {
    return `/ui/b/${verdict.invocationId.substring('build-'.length)}/test-results?q=ID%3A${encodeURIComponent(verdict.testId)}`;
  }
  return `/ui/inv/${verdict.invocationId}/test-results?q=ID%3A${encodeURIComponent(verdict.testId)}`;
};

interface FailureReasonDisplayProps {
  invocationId: string;
  testId: string;
  variantHash: string;
}
const FailureReasonDisplay = ({
  invocationId,
  testId,
  variantHash,
}: FailureReasonDisplayProps) => {
  const client = useResultDbClient();
  const { data, error, isError, isPending } = useQuery(
    client.BatchGetTestVariants.query(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: `invocations/${invocationId}`,
        testVariants: [
          {
            testId,
            variantHash,
          },
        ],
      }),
    ),
  );
  if (isPending) {
    return <LinearProgress />;
  }
  if (isError) {
    return (
      <div css={{ color: 'var(--failure-color)' }}>
        {(error as Error).message}
      </div>
    );
  }
  const failureReason = data?.testVariants[0].results.find(
    (r) => !r.result?.expected && r.result?.failureReason?.primaryErrorMessage,
  )?.result?.failureReason?.primaryErrorMessage;

  if (failureReason) {
    return <pre css={{ whiteSpace: 'pre-wrap' }}>{failureReason}</pre>;
  }
  return null;
};
