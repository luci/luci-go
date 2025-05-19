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
import CodeIcon from '@mui/icons-material/Code';
import CommitIcon from '@mui/icons-material/Commit';
import ComputerIcon from '@mui/icons-material/Computer';
import { Box, ButtonBase, Link, Typography } from '@mui/material';
import React, {
  cloneElement,
  Fragment,
  useCallback,
  useMemo,
  useState,
} from 'react';

import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import {
  getCommitInfoFromInvocation,
  getInvocationTag,
  getVariantValue,
} from '@/test_investigation/utils/test_info_utils';

import { CLsPopover } from './cls_popver';
import { useFormattedCLs } from './context';
import { InfoLineItem, NO_CLS_TEXT } from './types';

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
  const infoLineItems = useMemo((): InfoLineItem[] => {
    // TODO: this is currently hardcoded to specific keys, it needs to be generic.
    const items: InfoLineItem[] = [];
    const builder = getVariantValue(testVariant?.variant, 'builder');
    const testSuiteNameFromVariant = getVariantValue(
      testVariant?.variant,
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
              {NO_CLS_TEXT}
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

  const testDisplayName =
    testVariant?.testMetadata?.name || testVariant?.testId;

  return (
    <Box sx={{ mb: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
        <Typography
          variant="h5"
          component="h1"
          sx={{ color: 'var(--gm3-color-on-surface-strong)', mr: 0.5 }}
        >
          Test case:
        </Typography>
        <Typography
          variant="h5"
          component="span"
          sx={{
            color: 'var(--gm3-color-on-surface-strong)',
            wordBreak: 'break-all',
          }}
        >
          {testDisplayName}
        </Typography>
        <CopyToClipboard
          textToCopy={testDisplayName}
          aria-label="Copy test ID."
          sx={{ ml: 0.5 }}
        />
      </Box>
      {infoLineItems.length > 0 && (
        <Typography
          component="div"
          variant="body2"
          sx={{ mt: 0.5, display: 'flex', flexWrap: 'wrap', gap: '4px 0px' }}
        >
          {infoLineItems.map((item, index) => (
            <Fragment key={item.label + index}>
              {index > 0 && (
                <Box
                  component="span"
                  sx={{
                    mx: 0.75,
                    color: 'text.disabled',
                    alignSelf: 'center',
                    lineHeight: 'initial',
                  }}
                >
                  |
                </Box>
              )}
              <Box
                component="span"
                sx={{ display: 'inline-flex', alignItems: 'center' }}
              >
                {item.icon &&
                  cloneElement(item.icon, {
                    sx: { fontSize: '1rem', mr: 0.5, color: 'text.secondary' },
                  })}
                <Typography
                  variant="body2"
                  component="span"
                  color="text.secondary"
                >
                  {item.label}:
                </Typography>
                {item.customRender ? (
                  item.customRender()
                ) : item.href && !item.isPlaceholder ? (
                  <Link
                    href={item.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    underline="hover"
                    variant="body2"
                    color="text.primary"
                    sx={{ ml: 0.5 }}
                  >
                    {item.value}
                  </Link>
                ) : (
                  <Typography
                    variant="body2"
                    component="span"
                    color={
                      item.isPlaceholder ? 'text.disabled' : 'text.primary'
                    }
                    sx={{ ml: 0.5 }}
                  >
                    {item.value}
                  </Typography>
                )}
              </Box>
            </Fragment>
          ))}
        </Typography>
      )}
      <CLsPopover
        anchorEl={clPopoverAnchorEl}
        open={clPopoverOpen}
        onClose={handleCLPopoverClose}
        id={clPopoverId}
      />
    </Box>
  );
}
