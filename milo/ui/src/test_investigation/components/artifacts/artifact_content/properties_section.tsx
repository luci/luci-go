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

import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';

import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { useTestVariant } from '@/test_investigation/context';

import { CollapsibleArtifactSummarySection } from './collapsible_artifact_summary_section';

interface PropertiesSectionProps {
  currentResult: TestResult;
}

export function PropertiesSection({ currentResult }: PropertiesSectionProps) {
  const testVariant = useTestVariant();
  const hasVariantProperties =
    testVariant.variant && Object.keys(testVariant.variant.def).length > 0;
  const hasTags = currentResult.tags && currentResult.tags.length > 0;

  if (!hasVariantProperties && !hasTags) {
    return null;
  }

  return (
    <CollapsibleArtifactSummarySection
      title="Properties"
      helpText="Key-value pairs defining the properties of this test result, including module-level and test-specific properties."
    >
      <Box
        sx={{
          pl: 1,
          pr: 1,
          pb: 1,
        }}
      >
        {hasVariantProperties && (
          <>
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
                Module Properties
              </Typography>
              <Tooltip
                title={`Properties that are set at the module level, and are the same for all tests in the module.
                  These properties form part of the tests identity, so a test with the same name, but different
                  module properties will be treated as a different test history.`}
              >
                <IconButton
                  size="small"
                  sx={{ ml: 0.5 }}
                  aria-label="Info for Module Properties"
                >
                  <HelpOutlineIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
            {Object.entries(testVariant.variant!.def).map(([key, value]) => (
              <Box key={key} sx={{ display: 'flex', mb: 0.5 }}>
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 'medium',
                    color: 'text.secondary',
                    mr: 1,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {key}:
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    color: 'text.primary',
                    wordBreak: 'break-word',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {value}
                </Typography>
              </Box>
            ))}
          </>
        )}

        {hasTags && (
          <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', mt: 4 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
                Test Properties
              </Typography>
              <Tooltip
                title={`Properties specific to this individual test result attempt.  These have no impact on test identity or history.`}
              >
                <IconButton
                  size="small"
                  sx={{ ml: 0.5 }}
                  aria-label="Info for Test Properties"
                >
                  <HelpOutlineIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
            {currentResult.tags.map((tag) => (
              <Box key={tag.key} sx={{ display: 'flex', mb: 0.5 }}>
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 'medium',
                    color: 'text.secondary',
                    mr: 1,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {tag.key}:
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    color: 'text.primary',
                    wordBreak: 'break-word',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {tag.value}
                </Typography>
              </Box>
            ))}
          </Box>
        )}
      </Box>
    </CollapsibleArtifactSummarySection>
  );
}
