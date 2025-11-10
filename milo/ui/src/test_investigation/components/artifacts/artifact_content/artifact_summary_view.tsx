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

import { Box, Button, Chip, Typography } from '@mui/material';
import copy from 'copy-to-clipboard';
import { useMemo } from 'react';

import { TestResultSummary } from '@/common/components/test_result_summary';
import { getStatusStyle } from '@/common/styles/status_styles';
import {
  displayCompactDuration,
  parseProtoDuration,
} from '@/common/tools/time_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { useIsLegacyInvocation } from '@/test_investigation/context/context';

import { CollapsibleArtifactSummarySection } from './collapsible_artifact_summary_section';
import { LegacyInvocationLinks } from './legacy_invocation_links';
import { PropertiesSection } from './properties_section';
import { TextDiffArtifactView } from './text_diff_artifact_view';

interface ArtifactSummaryViewProps {
  currentResult: TestResult;
  textDiffArtifact?: Artifact;
  selectedAttemptIndex: number;
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
  selectedAttemptIndex,
}: ArtifactSummaryViewProps) {
  const isLegacyInvocation = useIsLegacyInvocation();
  const failureStatusStyle = useMemo(() => getStatusStyle('failed'), []);
  const neutralStatusStyle = useMemo(() => getStatusStyle('neutral'), []);
  const failureKindAsMessage = useMemo(
    () => getFailureReasonKindDisplayText(currentResult.failureReason?.kind),
    [currentResult.failureReason?.kind],
  );
  const hasStackTraces = useMemo(
    () => currentResult.failureReason?.errors?.some((e) => e.trace) ?? false,
    [currentResult.failureReason?.errors],
  );

  const [compactDuration] =
    (currentResult.duration &&
      displayCompactDuration(parseProtoDuration(currentResult.duration))) ||
    [];

  return (
    <>
      <Box
        sx={{
          pl: 1,
          pr: 1,
          pb: 1,
          display: 'flex',
          alignItems: 'center',
          gap: 1,
        }}
      >
        {compactDuration && (
          <Chip label={compactDuration} color="primary"></Chip>
        )}
        {isLegacyInvocation && (
          <LegacyInvocationLinks
            currentResult={currentResult}
            selectedAttemptIndex={selectedAttemptIndex}
          />
        )}
      </Box>
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
      {hasStackTraces && (
        <CollapsibleArtifactSummarySection
          title="Stack trace(s)"
          helpText="Stack traces associated with the test failure."
        >
          <Box sx={{ pl: 1, pr: 1, pb: 1 }}>
            {currentResult
              .failureReason!.errors.filter((e) => e.trace)
              .map((error, index) => (
                <>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <Button
                      variant="outlined"
                      size="small"
                      sx={{ mb: 1.5 }}
                      onClick={() => {
                        copy(error.trace);
                      }}
                    >
                      Copy to clipboard
                    </Button>
                  </Box>

                  <Box
                    key={index}
                    sx={{
                      p: 1,
                      border: `1px solid ${neutralStatusStyle.borderColor}`,
                      borderRadius: '4px',
                      backgroundColor: neutralStatusStyle.backgroundColor,
                      '&:not(:last-child)': {
                        mb: 1,
                      },
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
                      {error.trace}
                    </Typography>
                  </Box>
                </>
              ))}
          </Box>
        </CollapsibleArtifactSummarySection>
      )}
      <CollapsibleArtifactSummarySection
        title="Summary"
        helpText="A HTML explanation of the test result generated by the test uploading code."
      >
        <Box sx={{ pl: 1, pr: 1, pb: 1 }}>
          {currentResult.summaryHtml ? (
            <TestResultSummary testResult={currentResult} />
          ) : (
            <Typography variant="body2" sx={{ fontStyle: ' italic' }}>
              No summary available.
            </Typography>
          )}
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
