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

import CodeIcon from '@mui/icons-material/Code';
import CommitIcon from '@mui/icons-material/Commit';
import { Box, ButtonBase, Link } from '@mui/material';
import React, { useCallback, useState } from 'react';

import {
  PageSummaryLine,
  SummaryLineItem,
} from '@/common/components/page_summary_line';
import { PageTitle } from '@/common/components/page_title';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import {
  getCommitGitilesUrlFromInvocation,
  getCommitInfoFromInvocation,
} from '@/test_investigation/utils/test_info_utils';

import { CLsPopover } from './cls_popver';
import { useFormattedCLs } from './context';
import { NO_CLS_TEXT } from './types';

export function TestInfoHeader() {
  const testVariant = useTestVariant();
  const invocation = useInvocation();
  const allFormattedCLs = useFormattedCLs();
  const [clPopoverAnchorEl, setClPopoverAnchorEl] =
    useState<HTMLElement | null>(null);
  const handleCLPopoverOpen = useCallback(
    (event: React.MouseEvent<HTMLElement>) =>
      setClPopoverAnchorEl(event.currentTarget),
    [],
  );
  const handleCLPopoverClose = useCallback(() => {
    setClPopoverAnchorEl(null);
  }, []);

  const clPopoverOpen = Boolean(clPopoverAnchorEl);
  const clPopoverId = clPopoverOpen ? 'cl-popover-main-test-info' : undefined;
  const testDisplayName =
    testVariant?.testMetadata?.name || testVariant?.testId;
  const commitInfo = getCommitInfoFromInvocation(invocation);
  const commitLink = getCommitGitilesUrlFromInvocation(invocation);

  return (
    <Box sx={{ mb: 2 }}>
      <PageTitle viewName="Test case" resourceName={testDisplayName} />
      <PageSummaryLine>
        {Object.entries(testVariant.variant?.def || {}).map(([key, value]) => (
          <SummaryLineItem key={key} label={key}>
            {value}
          </SummaryLineItem>
        ))}
        <SummaryLineItem label="Commit" icon={<CommitIcon />}>
          <Link href={commitLink} target="_blank" rel="noopener noreferrer">
            {commitInfo}
          </Link>
        </SummaryLineItem>
        <SummaryLineItem label="CL" icon={<CodeIcon />}>
          {allFormattedCLs.length === 0 ? (
            NO_CLS_TEXT
          ) : (
            <>
              <Link
                href={allFormattedCLs[0].url}
                target="_blank"
                rel="noopener noreferrer"
              >
                {allFormattedCLs[0].display}
              </Link>
              {allFormattedCLs.length > 1 && (
                <ButtonBase
                  onClick={handleCLPopoverOpen}
                  aria-describedby={clPopoverId}
                  sx={{
                    ml: 0.5,
                    textDecoration: 'underline',
                    color: 'primary.main',
                    cursor: 'pointer',
                    typography: 'body2',
                  }}
                >
                  + {allFormattedCLs.length - 1} more
                </ButtonBase>
              )}
            </>
          )}
        </SummaryLineItem>
      </PageSummaryLine>
      <CLsPopover
        anchorEl={clPopoverAnchorEl}
        open={clPopoverOpen}
        onClose={handleCLPopoverClose}
        id={clPopoverId}
      />
    </Box>
  );
}
