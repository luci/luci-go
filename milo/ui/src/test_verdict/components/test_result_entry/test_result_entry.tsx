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
import { useState } from 'react';
import { Link as RouterLink } from 'react-router';

import { DurationBadge } from '@/common/components/duration_badge';
import { TagsEntry } from '@/common/components/tags_entry';
import {
  TEST_STATUS_V2_CLASS_MAP,
  testResultStatusLabel,
} from '@/common/constants/verdict';
import { parseInvId } from '@/common/tools/invocation_utils';
import { parseTestResultName } from '@/common/tools/test_result_utils/index';
import { parseProtoDuration } from '@/common/tools/time_utils';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import {
  TestResult,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { ArtifactsEntry } from './artifacts_entry';
import { FailureReasonEntry } from './failure_reason_entry';
import { ResultAssociatedBugsBadge } from './result_associated_bugs_badge';
import { SummaryHtmlEntry } from './summary_html_entry';

export interface TestResultEntryProps {
  readonly index: number;
  readonly project?: string;
  readonly testResult: TestResult;
  /**
   * A fallback test ID when `testResult.testId === ''`.
   *
   * When a test result is included in a test verdict, some common properties
   * (e.g. test_id, variant, variant_hash, source_metadata) are striped by the
   * RPC to reduce the size of the response payload. This property provides an
   * alternative way to specify the test ID.
   */
  readonly testId?: string;
  /**
   * Whether the entry should expand when it's first rendered.
   */
  readonly initExpanded?: boolean;
}

export function TestResultEntry({
  index,
  project,
  testResult,
  testId,
  initExpanded = false,
}: TestResultEntryProps) {
  const [expanded, setExpanded] = useState(initExpanded);

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
        {testResult.statusV2 === TestResult_Status.SKIPPED ||
        testResult.statusV2 === TestResult_Status.PRECLUDED
          ? 'was '
          : ''}
        <span className={TEST_STATUS_V2_CLASS_MAP[testResult.statusV2]}>
          {testResultStatusLabel(testResult)}
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
        {project && (
          <>
            {' '}
            <ResultAssociatedBugsBadge
              project={project}
              testResult={testResult}
              testId={testId}
            />
          </>
        )}
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {testResult.failureReason && (
          <FailureReasonEntry failureReason={testResult.failureReason} />
        )}
        <SummaryHtmlEntry testResult={testResult} />
        <ArtifactsEntry testResultName={testResult.name} />
        {testResult.tags.length > 0 && <TagsEntry tags={testResult.tags} />}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
