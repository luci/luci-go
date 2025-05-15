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

import BuildIcon from '@mui/icons-material/Build';
import CategoryIcon from '@mui/icons-material/Category';
import CodeIcon from '@mui/icons-material/Code';
import CommitIcon from '@mui/icons-material/Commit';
import ComputerIcon from '@mui/icons-material/Computer';
import {
  Box,
  Link,
  Typography,
  Popover,
  List,
  ListItemButton,
  ListItemText,
  ButtonBase,
} from '@mui/material';
import React, { JSX, useCallback, useMemo, useState } from 'react';

import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { TestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import {
  getInvocationTag,
  getCommitInfoFromInvocation,
  getVariantValue,
  formatAllCLs,
  analyzeSegments,
  formatDurationSince,
} from '../../utils/test_info_utils';

import { OverviewSection } from './overview_section';
import { RecommendationsSection } from './recommendations_section';
import { TestInfoHeader } from './test_info_header';
import { InfoLineItem } from './types';

export interface TestInfoProps {
  invocation: Invocation;
  testVariant: TestVariant;
  associatedBugs?: readonly AssociatedBug[];
  testVariantBranch?: TestVariantBranch | null;
  bisectionAnalysis?: TestAnalysis | null;
}

export function TestInfo({
  invocation,
  testVariant,
  associatedBugs,
  testVariantBranch,
  bisectionAnalysis,
}: TestInfoProps): JSX.Element {
  const testDisplayName = testVariant.testMetadata?.name || testVariant.testId;

  const allFormattedCLs = useMemo(
    () => formatAllCLs(invocation.sourceSpec?.sources?.changelists),
    [invocation.sourceSpec?.sources?.changelists],
  );

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

  const infoLineItems = useMemo((): InfoLineItem[] => {
    // TODO: this is currently hardcoded to specific keys, it needs to be generic.
    const items: InfoLineItem[] = [];
    const builder = getVariantValue(testVariant.variant, 'builder');
    const testSuiteNameFromVariant = getVariantValue(
      testVariant.variant,
      'test_suite',
    );
    const testSuiteNameFromTestId = testVariant.testId
      ? testVariant.testId.split('.')[0]
      : undefined;
    const testSuiteNameFromInvTag = getInvocationTag(
      invocation.tags,
      'test_suite',
    );
    const testSuiteName =
      testSuiteNameFromVariant ||
      testSuiteNameFromTestId ||
      testSuiteNameFromInvTag;
    const osFromInvTag =
      getInvocationTag(invocation.tags, 'os') ||
      getInvocationTag(invocation.tags, 'os_family');
    const osName = osFromInvTag || getVariantValue(testVariant.variant, 'os');
    const commitInfo = getCommitInfoFromInvocation(invocation);

    if (builder)
      items.push({ label: 'Builder', value: builder, icon: <BuildIcon /> });
    if (testSuiteName)
      items.push({
        label: 'Test suite',
        value: testSuiteName,
        icon: <CategoryIcon />,
      });
    if (osName)
      items.push({ label: 'OS', value: osName, icon: <ComputerIcon /> });
    if (commitInfo)
      items.push({ label: 'Commit', value: commitInfo, icon: <CommitIcon /> });
    items.push({
      label: 'CL',
      icon: <CodeIcon />,
      customRender: () => {
        if (allFormattedCLs.length === 0) {
          return (
            <Typography
              variant="body2"
              component="span"
              color="text.disabled"
              sx={{ ml: 0.5 }}
            >
              No CLs patched for this test.
            </Typography>
          );
        }
        const firstCL = allFormattedCLs[0];
        return (
          <>
            <Link
              href={firstCL.url}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
              variant="body2"
              color="text.primary"
              sx={{ ml: 0.5 }}
            >
              {firstCL.display}
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
        );
      },
    });
    return items.filter(
      (item) => item.isPlaceholder === false || item.customRender || item.value,
    );
  }, [
    testVariant,
    invocation,
    allFormattedCLs,
    handleCLPopoverOpen,
    clPopoverId,
  ]);

  const segmentAnalysis = useMemo(
    () => analyzeSegments(testVariantBranch?.segments),
    [testVariantBranch],
  );

  // Data points that were part of uiPlaceholders, now defined directly or passed
  const recommendationCount = associatedBugs?.length || 0;
  const lastUpdateTime = invocation.finalizeTime
    ? formatDurationSince(invocation.finalizeTime) || 'N/A'
    : 'N/A';
  // Define any other *real* links or data points that were in uiPlaceholders and are still needed
  const allTestHistoryLink = '#all-history-todo'; // Keep if this is a real link target
  const clHistoryLinkSuffix = '/+log';
  const compareLink = '#compare-todo';

  return (
    <>
      <TestInfoHeader
        testDisplayName={testDisplayName}
        infoLineItems={infoLineItems}
        allFormattedCLs={allFormattedCLs}
      />
      <Popover
        id={clPopoverId}
        open={clPopoverOpen}
        anchorEl={clPopoverAnchorEl}
        onClose={handleCLPopoverClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
      >
        <List dense sx={{ minWidth: 250, maxWidth: 500, p: 1 }}>
          {allFormattedCLs.map((cl) => (
            <ListItemButton
              key={cl.key}
              component="a"
              href={cl.url}
              target="_blank"
              rel="noopener noreferrer"
              sx={{ pl: 1, pr: 1, py: 0.5 }}
            >
              <ListItemText
                primary={cl.display}
                primaryTypographyProps={{
                  variant: 'body2',
                  style: {
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  },
                }}
              />
            </ListItemButton>
          ))}
        </List>
      </Popover>

      <Box
        sx={{
          display: 'flex',
          flexDirection: { xs: 'column', md: 'row' },
          gap: '32px',
          mb: 2,
        }}
      >
        <OverviewSection
          invocation={invocation}
          testVariant={testVariant}
          associatedBugs={associatedBugs}
          bisectionAnalysis={bisectionAnalysis}
          segmentAnalysis={segmentAnalysis}
          allFormattedCLs={allFormattedCLs}
          onCLPopoverOpen={handleCLPopoverOpen}
          allTestHistoryLink={allTestHistoryLink}
          clHistoryLinkSuffix={clHistoryLinkSuffix}
          compareLink={compareLink}
        />
        <RecommendationsSection
          recommendationCount={recommendationCount}
          lastUpdateTime={lastUpdateTime}
          segmentAnalysis={segmentAnalysis}
          associatedBugs={associatedBugs}
          allTestHistoryLink={allTestHistoryLink}
        />
      </Box>
    </>
  );
}
