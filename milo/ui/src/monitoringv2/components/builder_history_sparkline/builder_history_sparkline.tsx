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

import { Link } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { BUILD_STATUS_CLASS_MAP } from '@/build/constants';
import { SpecifiedStatus } from '@/build/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import { OneBuildHistory } from '@/monitoringv2/pages/monitoring_page/context/context';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { statusToJSON } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface BuilderHistorySparklineProps {
  builderId: BuilderID;
  history: OneBuildHistory[];
  numHighlighted: number;
}

export const BuilderHistorySparkline = ({
  builderId,
  history,
  numHighlighted,
}: BuilderHistorySparklineProps) => {
  return (
    <div css={{ display: 'flex' }}>
      {history.map((build, i) => (
        <HtmlTooltip
          key={build.buildId}
          title={
            <>
              <div>
                <span
                  css={{ fontWeight: '700' }}
                  className={
                    BUILD_STATUS_CLASS_MAP[build.status as SpecifiedStatus] ||
                    ''
                  }
                >
                  {build.status === undefined // This can be undefined if the step didn't execute in this build.
                    ? 'NOT RUN'
                    : statusToJSON(build.status)}
                </span>{' '}
                {build.startTime ? (
                  <RelativeTimestamp
                    timestamp={DateTime.fromISO(build.startTime!)}
                  />
                ) : null}
              </div>
              <SummaryMarkdownDisplay summaryMarkdown={build.summaryMarkdown} />
            </>
          }
        >
          <Link
            href={`/ui/b/${build.buildId}`}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            <div
              key={build.buildId}
              className={`${BUILD_STATUS_CLASS_MAP[build.status as SpecifiedStatus]}-bg-pattern`}
              css={{
                width: '11px',
                height: '18px',
                opacity: i < numHighlighted ? 1 : 0.5,
              }}
            ></div>
          </Link>
        </HtmlTooltip>
      ))}
      <HtmlTooltip title="View full builder history">
        <Link
          href={`/ui/p/${builderId.project}/builders/${builderId.bucket}/${builderId.builder}`}
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

interface SummaryMarkdownDisplayProps {
  summaryMarkdown: string | undefined;
}

const SummaryMarkdownDisplay = ({
  summaryMarkdown,
}: SummaryMarkdownDisplayProps) => {
  const summaryHtml = useMemo(
    () => (summaryMarkdown ? renderMarkdown(summaryMarkdown) : null),
    [summaryMarkdown],
  );
  if (!summaryHtml) {
    return null;
  }
  return (
    <div
      css={{
        padding: '5px 0',
        clear: 'both',
        overflowWrap: 'break-word',
        '& pre': {
          whiteSpace: 'pre-wrap',
          overflowWrap: 'break-word',
          fontSize: '12px',
        },
        '& *': {
          marginBlock: '10px',
        },
      }}
    >
      <SanitizedHtml html={summaryHtml} />
    </div>
  );
};
