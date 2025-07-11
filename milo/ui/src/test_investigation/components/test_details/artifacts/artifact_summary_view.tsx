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

import { Box, Typography } from '@mui/material';
import { useMemo } from 'react';

import { TestResultSummary } from '@/common/components/test_result_summary';
import { getStatusStyle } from '@/common/styles/status_styles';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { CollapsibleArtifactSummarySection } from './collapsible_artifact_summary_section';
import { PropertiesSection } from './properties_section';
import { TextDiffArtifactView } from './text_diff_artifact_view';

interface ArtifactSummaryViewProps {
  currentResult: TestResult;
  textDiffArtifact?: Artifact;
}

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
}: ArtifactSummaryViewProps) {
  const failureStatusStyle = useMemo(() => getStatusStyle('failed'), []);
  const neutralStatusStyle = useMemo(() => getStatusStyle('neutral'), []);
  const failureKindAsMessage = useMemo(
    () => getFailureReasonKindDisplayText(currentResult.failureReason?.kind),
    [currentResult.failureReason?.kind],
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
