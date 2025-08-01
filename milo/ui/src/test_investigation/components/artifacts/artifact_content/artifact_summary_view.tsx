// Copyright 2025 The LUCI Authors.
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

import { Box, Link, Typography } from '@mui/material';
import { useMemo } from 'react';

import { TestResultSummary } from '@/common/components/test_result_summary';
import {
  getStatusStyle,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import { parseInvId } from '@/common/tools/invocation_utils';
import { parseTestResultName } from '@/common/tools/test_result_utils';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import {
  SkippedReason_Kind,
  TestResult,
  TestResult_Status,
  WebTest_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { CollapsibleArtifactSummarySection } from './collapsible_artifact_summary_section';
import { PropertiesSection } from './properties_section';
import { TextDiffArtifactView } from './text_diff_artifact_view';

interface ArtifactSummaryViewProps {
  currentResult: TestResult;
  textDiffArtifact?: Artifact;
  selectedAttemptIndex: number;
}

const TEST_STATUS_V2_DISPLAY_MAP = Object.freeze({
  [TestResult_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestResult_Status.FAILED]: 'failed',
  [TestResult_Status.PASSED]: 'passed',
  [TestResult_Status.SKIPPED]: 'skipped',
  [TestResult_Status.EXECUTION_ERRORED]: 'execution errored',
  [TestResult_Status.PRECLUDED]: 'precluded',
});
const WEB_TEST_STATUS_DISPLAY_MAP = Object.freeze({
  [WebTest_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [WebTest_Status.FAIL]: 'failed',
  [WebTest_Status.PASS]: 'passed',
  [WebTest_Status.SKIP]: 'skipped',
  [WebTest_Status.CRASH]: 'crashed',
  [WebTest_Status.TIMEOUT]: 'timed out',
});

function getFailureReasonKindDisplayText(
  kind?: FailureReason_Kind,
): string | undefined {
  if (
    kind === undefined ||
    kind === FailureReason_Kind.KIND_UNSPECIFIED ||
    kind === FailureReason_Kind.ORDINARY
  ) {
    return undefined;
  }
  const kindStr = failureReason_KindToJSON(kind);
  if (kindStr === 'KIND_UNSPECIFIED' || kindStr === 'ORDINARY') {
    return undefined;
  }
  return kindStr
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

export function ArtifactSummaryView({
  currentResult,
  textDiffArtifact,
  selectedAttemptIndex,
}: ArtifactSummaryViewProps) {
  const failureStatusStyle = useMemo(() => getStatusStyle('failed'), []);
  const neutralStatusStyle = useMemo(() => getStatusStyle('neutral'), []);
  const failureKindAsMessage = useMemo(
    () => getFailureReasonKindDisplayText(currentResult.failureReason?.kind),
    [currentResult.failureReason?.kind],
  );

  const renderStatus = () => {
    const requiresLeadingWas =
      currentResult.statusV2 === TestResult_Status.PRECLUDED ||
      currentResult.statusV2 === TestResult_Status.SKIPPED;
    const webTest = currentResult.frameworkExtensions?.webTest;
    const failureKind =
      currentResult.failureReason?.kind || FailureReason_Kind.KIND_UNSPECIFIED;
    const skippedKind =
      currentResult.skippedReason?.kind || SkippedReason_Kind.KIND_UNSPECIFIED;

    let statusDetail: string = '';
    if (webTest) {
      statusDetail = `${webTest.isExpected ? 'expectedly' : 'unexpectedly'} ${WEB_TEST_STATUS_DISPLAY_MAP[webTest.status]}`;
    } else if (
      currentResult.statusV2 === TestResult_Status.FAILED &&
      failureKind !== FailureReason_Kind.KIND_UNSPECIFIED
    ) {
      switch (failureKind) {
        case FailureReason_Kind.CRASH:
          statusDetail = 'crashed';
          break;
        case FailureReason_Kind.TIMEOUT:
          statusDetail = 'timed out';
          break;
        case FailureReason_Kind.ORDINARY:
          // No detail to show.
          break;
      }
    } else if (
      currentResult.statusV2 === TestResult_Status.SKIPPED &&
      skippedKind !== SkippedReason_Kind.KIND_UNSPECIFIED
    ) {
      switch (skippedKind) {
        case SkippedReason_Kind.DEMOTED:
          statusDetail = 'demoted';
          break;
        case SkippedReason_Kind.DISABLED_AT_DECLARATION:
          statusDetail = 'disabled at declaration';
          break;
        case SkippedReason_Kind.SKIPPED_BY_TEST_BODY:
          statusDetail = 'by test body';
          break;
        case SkippedReason_Kind.OTHER:
          // No status detail to show, but the failure reason section will contain information.
          break;
      }
    }

    return `${requiresLeadingWas ? 'was ' : ''} ${TEST_STATUS_V2_DISPLAY_MAP[currentResult.statusV2]}
      ${statusDetail ? ` (${statusDetail})` : ''}
    `;
  };

  const parentInvId = parseTestResultName(currentResult.name).invocationId;
  const parsedInvId = parseInvId(parentInvId);
  const testResultStyle = getStatusStyle(
    TEST_STATUS_V2_DISPLAY_MAP[currentResult.statusV2] as SemanticStatusType,
  );
  return (
    <>
      {currentResult.failureReason && (
        <CollapsibleArtifactSummarySection
          title="Failure Reason"
          helpText="The primary error message for this test failure, as selected by the test uploading code."
        >
          {currentResult.failureReason.primaryErrorMessage ||
          failureKindAsMessage ? (
            <Box sx={{ pl: 1, pr: 1, pb: 1 }}>
              <Box
                sx={{
                  p: 1,
                  border: `1px solid ${failureStatusStyle.borderColor}`,
                  borderRadius: '4px',
                  backgroundColor: neutralStatusStyle.backgroundColor,
                }}
              >
                <Typography
                  variant="body2"
                  sx={{
                    whiteSpace: 'pre-wrap',
                    fontFamily: 'monospace',
                    color: neutralStatusStyle.textColor,
                  }}
                >
                  {currentResult.failureReason.primaryErrorMessage ||
                    failureKindAsMessage}
                </Typography>
              </Box>
            </Box>
          ) : (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ pl: 1, pr: 1, pb: 1, fontStyle: 'italic' }}
            >
              No primary error message uploaded.
            </Typography>
          )}
          <Box sx={{ pl: 1, pr: 1, pb: 1 }}>
            {parsedInvId.type === 'swarming-task' && (
              <Typography variant="body2">
                Result #{selectedAttemptIndex + 1}{' '}
                <span
                  style={{
                    color: testResultStyle.textColor,
                  }}
                >
                  {renderStatus()}{' '}
                </span>
                in task:{' '}
                <Link
                  href={getSwarmingTaskURL(
                    parsedInvId.swarmingHost,
                    parsedInvId.taskId,
                  )}
                  target="_blank"
                >
                  {parsedInvId.taskId}
                </Link>
              </Typography>
            )}
            {parsedInvId.type === 'build' && (
              <Typography variant="body2" sx={{ mt: 2 }}>
                Result #{selectedAttemptIndex + 1}{' '}
                <span
                  style={{
                    color: testResultStyle.textColor,
                  }}
                >
                  {renderStatus()}{' '}
                </span>
                in build:{' '}
                <Link href={`/ui/b/${parsedInvId.buildId}`} target="_blank">
                  {parsedInvId.buildId}
                </Link>
              </Typography>
            )}
          </Box>
        </CollapsibleArtifactSummarySection>
      )}
      <CollapsibleArtifactSummarySection
        title="Summary"
        helpText="A HTML explanation of the test result generated by the test uploading code."
      >
        <Box sx={{ pl: 1, pr: 1, pb: 1 }}>
          <TestResultSummary testResult={currentResult} />
        </Box>
      </CollapsibleArtifactSummarySection>
      {textDiffArtifact && (
        <CollapsibleArtifactSummarySection
          title="Text Diff"
          helpText="A diff of the expected and actual text output of the test. Most often used with browser WPT tests."
        >
          <Box sx={{ pl: 1, pr: 1, pb: 1 }}>
            <TextDiffArtifactView artifact={textDiffArtifact} />
          </Box>
        </CollapsibleArtifactSummarySection>
      )}

      <PropertiesSection currentResult={currentResult} />
    </>
  );
}
