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
import { useRef, useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';

import { DurationBadge } from '@/common/components/duration_badge';
import { TagsEntry } from '@/common/components/tags_entry';
import { TEST_STATUS_DISPLAY_MAP } from '@/common/constants/test';
import { parseProtoDuration } from '@/common/tools/time_utils';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { parseInvId } from '@/test_verdict/tools/invocation_utils';
import { parseTestResultName } from '@/test_verdict/tools/utils';

import { ArtifactsEntry } from './artifacts_entry';
import { FailureReasonEntry } from './failure_reason_entry';
import { SummaryHtmlEntry } from './summary_html_entry';

export interface TestResultEntryProps {
  readonly index: number;
  readonly testResult: TestResult;
}

export function TestResultEntry({ index, testResult }: TestResultEntryProps) {
  const [expanded, setExpanded] = useState(false);

  // `wasExpanded` does not need to be a state. It can only be updated when
  // another state, `expanded`, is updated. Making it a state can cause
  // unnecessary rerendering.
  const wasExpandedRef = useRef(false);
  wasExpandedRef.current ||= expanded;

  const parsedName = parseTestResultName(testResult.name);
  const parsedInvId = parseInvId(parsedName.invocationId);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={(expand) => setExpanded(expand)}>
        <DurationBadge
          duration={
            testResult.duration && parseProtoDuration(testResult.duration)
          }
        />{' '}
        result #{index + 1}{' '}
        <span className={testResult.expected ? 'expected' : 'unexpected'}>
          {testResult.expected ? 'expectedly' : 'unexpectedly'}{' '}
          {TEST_STATUS_DISPLAY_MAP[testResult.status]}
        </span>
        {parsedInvId.type === 'swarming-task' && (
          <>
            {' '}
            in task:{' '}
            <Link
              href={getSwarmingTaskURL(
                parsedInvId.swarmingHost,
                parsedInvId.taskId,
              )}
              target="_blank"
              rel="noopenner"
              onClick={(e) => e.stopPropagation()}
            >
              {parsedInvId.taskId}
            </Link>
          </>
        )}
        {parsedInvId.type === 'build' && (
          <>
            {' '}
            in build:{' '}
            <Link
              component={RouterLink}
              to={`/ui/b/${parsedInvId.buildId}`}
              target="_blank"
              rel="noopenner"
              onClick={(e) => e.stopPropagation()}
            >
              {parsedInvId.buildId}
            </Link>
          </>
        )}
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {wasExpandedRef.current && (
          <>
            {testResult.failureReason && (
              <FailureReasonEntry failureReason={testResult.failureReason} />
            )}
            <SummaryHtmlEntry testResult={testResult} />
            <ArtifactsEntry testResultName={testResult.name} />
            {testResult.tags.length > 0 && <TagsEntry tags={testResult.tags} />}
          </>
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
