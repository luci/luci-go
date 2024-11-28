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

import Link from '@mui/material/Link';
import { DateTime } from 'luxon';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { OneTestHistory } from '@/monitoringv2/pages/monitoring_page/context/context';
import { TestVerdictStatus } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { testVariantStatusToJSON } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

const VARIANT_STATUS_CLASS_MAP: { [status: string]: string } = {
  [TestVerdictStatus.UNEXPECTED]: 'failure',
  [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]: 'failure',
  [TestVerdictStatus.FLAKY]: 'started',
  [TestVerdictStatus.EXONERATED]: 'canceled',
  [TestVerdictStatus.EXPECTED]: 'success',
};

export interface TestHistorySparklineProps {
  project: string;
  testId: string;
  variantHash: string;
  history: (OneTestHistory | undefined)[];
  numHighlighted: number;
}

export const TestHistorySparkline = ({
  project,
  testId,
  variantHash,
  history,
}: TestHistorySparklineProps) => {
  return (
    <div css={{ display: 'flex' }}>
      {history.map((history, i) => (
        <HtmlTooltip
          key={i}
          title={
            <>
              <div>
                <span
                  css={{ fontWeight: '700' }}
                  className={
                    (history && VARIANT_STATUS_CLASS_MAP[status]) || ''
                  }
                >
                  {history?.status && testVariantStatusToJSON(history.status)}
                </span>{' '}
                {history?.startTime ? (
                  <RelativeTimestamp
                    timestamp={DateTime.fromISO(history.startTime)}
                  />
                ) : null}
              </div>
              <div>
                <pre css={{ whiteSpace: 'pre-wrap' }}>
                  {history?.failureReason}
                </pre>
              </div>
            </>
          }
        >
          <Link
            href={testVerdictLink(testId, history)}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            <div
              className={`${(history?.status && VARIANT_STATUS_CLASS_MAP[history.status]) || 'scheduled'}-bg-pattern`}
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
          href={`/ui/test/${project}/${encodeURIComponent(testId)}?q=VHash%3A${encodeURIComponent(variantHash)}`}
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

const testVerdictLink = (
  testId: string,
  verdict: OneTestHistory | undefined,
): string => {
  return `/ui/b/${verdict?.buildId}/test-results?q=ID%3A${encodeURIComponent(testId)}`;
};
