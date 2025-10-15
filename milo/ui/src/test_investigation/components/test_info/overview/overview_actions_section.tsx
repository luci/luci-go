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

import { Box, Button } from '@mui/material';
import { useMemo } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import {
  constructFileBugUrl,
  constructCodesearchUrl,
  getVariantValue,
  getSourcesFromInvocation,
} from '@/test_investigation/utils/test_info_utils';

import { AthButton } from './ath_button';
import { RerunButton } from './rerun_button';
import { SourceInfoTooltipContent } from './source_info_tooltip_content';

export function OverviewActionsSection() {
  const testVariant = useTestVariant();
  const invocation = useInvocation();

  const builder = getVariantValue(testVariant.variant, 'builder');

  const fileBugUrl = useMemo(
    () =>
      constructFileBugUrl(
        invocation,
        testVariant,
        builder,
        '[HOTLIST_ID_PLACEHOLDER]',
      ),
    [invocation, testVariant, builder],
  );

  const testLocation = testVariant.testMetadata?.location;
  const codesearchUrl = useMemo(
    () => constructCodesearchUrl(testLocation),
    [testLocation],
  );

  const sourceRef = getSourcesFromInvocation(invocation)?.gitilesCommit?.ref;

  // TODO(b/445559255): conditionally render 'test case' or 'module' in rerun button labels when module page available.
  return (
    <Box
      sx={{
        display: 'flex',
        gap: 1,
        flexWrap: 'wrap',
        justifyContent: 'flex-start',
      }}
    >
      <RerunButton />
      <AthButton />
      <Button
        variant="outlined"
        size="small"
        href={fileBugUrl}
        rel="noopener noreferrer"
        target="_blank"
      >
        File bug
      </Button>
      {codesearchUrl ? (
        <HtmlTooltip
          title={
            <SourceInfoTooltipContent
              testLocation={testLocation}
              sourceRef={sourceRef}
              codesearchUrl={codesearchUrl}
            />
          }
          arrow
        >
          <Button
            variant="outlined"
            size="small"
            href={codesearchUrl}
            rel="noopener noreferrer"
            target="_blank"
          >
            View source file
          </Button>
        </HtmlTooltip>
      ) : (
        <Button variant="outlined" size="small" disabled>
          View source file
        </Button>
      )}
    </Box>
  );
}
