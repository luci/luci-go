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

import {
  Box,
  ButtonBase,
  Link,
  List,
  ListItemButton,
  ListItemText,
  Popover,
  Typography,
} from '@mui/material';
import React, { cloneElement, Fragment, JSX, useState } from 'react';

import { CopyButton } from '@/common/components/copy_button';
import { FormattedCLInfo } from '@/test_investigation/utils/test_info_utils';

import { InfoLineItem, NO_CLS_TEXT } from './types';

interface TestInfoHeaderProps {
  testDisplayName: string;
  infoLineItems: readonly InfoLineItem[];
  allFormattedCLs: readonly FormattedCLInfo[];
}

export function TestInfoHeader({
  testDisplayName,
  infoLineItems,
  allFormattedCLs,
}: TestInfoHeaderProps): JSX.Element {
  const [clPopoverAnchorEl, setClPopoverAnchorEl] =
    useState<HTMLElement | null>(null);

  const handleCLPopoverOpen = (event: React.MouseEvent<HTMLElement>) => {
    setClPopoverAnchorEl(event.currentTarget);
  };

  const handleCLPopoverClose = () => {
    setClPopoverAnchorEl(null);
  };

  const clPopoverOpen = Boolean(clPopoverAnchorEl);
  const clPopoverId = clPopoverOpen ? 'cl-popover-header' : undefined;

  const enrichedInfoLineItems = infoLineItems.map((item) => {
    if (item.label === 'CL') {
      return {
        ...item,
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
      };
    }
    return item;
  });

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
        <CopyButton
          textToCopy={testDisplayName}
          tooltipPlacement="top"
          aria-label="copy test name"
          sx={{ ml: 0.5 }}
        />
      </Box>
      {enrichedInfoLineItems.length > 0 && (
        <Typography
          component="div"
          variant="body2"
          sx={{ mt: 0.5, display: 'flex', flexWrap: 'wrap', gap: '4px 0px' }}
        >
          {enrichedInfoLineItems.map((item, index) => (
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
    </Box>
  );
}
