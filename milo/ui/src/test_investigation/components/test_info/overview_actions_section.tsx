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

import BugReportIcon from '@mui/icons-material/BugReport';
import SourceIcon from '@mui/icons-material/Source';
import { Box, Button } from '@mui/material';
import { useMemo } from 'react';

import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import {
  constructFileBugUrl,
  constructCodesearchUrl,
  getVariantValue,
} from '../../utils/test_info_utils';

interface OverviewActionsSectionProps {
  invocation: Invocation;
  testVariant: TestVariant;
  compareLink: string;
  onRerunClick?: () => void;
}

export function OverviewActionsSection({
  invocation,
  testVariant,
  compareLink,
  onRerunClick,
}: OverviewActionsSectionProps): JSX.Element {
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

  const codesearchUrl = useMemo(
    () => constructCodesearchUrl(testVariant.testMetadata?.location),
    [testVariant.testMetadata?.location],
  );

  return (
    <Box
      sx={{
        mt: 'auto',
        pt: 2,
        display: 'flex',
        gap: 1,
        flexWrap: 'wrap',
        justifyContent: 'flex-start',
      }}
    >
      <Button variant="outlined" size="small" href={compareLink}>
        Add comparison
      </Button>
      <Button
        variant="outlined"
        size="small"
        onClick={
          onRerunClick || (() => alert('Rerun functionality to be implemented'))
        }
      >
        Rerun
      </Button>
      <Button
        variant="outlined"
        size="small"
        href={fileBugUrl}
        rel="noopener noreferrer"
        startIcon={<BugReportIcon />}
      >
        File bug
      </Button>
      {codesearchUrl ? (
        <Button
          variant="outlined"
          size="small"
          href={codesearchUrl}
          rel="noopener noreferrer"
          startIcon={<SourceIcon />}
        >
          View source file
        </Button>
      ) : (
        <Button
          variant="outlined"
          size="small"
          disabled
          startIcon={<SourceIcon />}
        >
          View source file
        </Button>
      )}
    </Box>
  );
}
