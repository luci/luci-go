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

import { Box, ButtonBase, Link, Typography } from '@mui/material';
import React, { useCallback, useState } from 'react';

import { CLsPopover } from '@/test_investigation/components/test_info/cls_popver'; // Adjust path if needed
import { NO_CLS_TEXT } from '@/test_investigation/components/test_info/types'; // Assuming types file path

import { useFormattedCLs } from '../context';

const clHistoryLinkSuffix = '/+log';

export function AssociatedCLsSection() {
  const formattedCLs = useFormattedCLs() || [];

  const [clPopoverAnchorEl, setClPopoverAnchorEl] =
    useState<HTMLElement | null>(null);
  const handleCLPopoverOpen = useCallback(
    (event: React.MouseEvent<HTMLElement>) => {
      setClPopoverAnchorEl(event.currentTarget);
    },
    [],
  );
  const handleCLPopoverClose = useCallback(() => {
    setClPopoverAnchorEl(null);
  }, []);
  const clPopoverOpen = Boolean(clPopoverAnchorEl);

  return (
    <Box>
      <Typography
        gutterBottom
        variant="body2"
        color="text.secondary"
        component="div"
      >
        CLs
      </Typography>
      {formattedCLs.length === 0 && (
        <Typography component="span" color="text.disabled">
          {NO_CLS_TEXT}
        </Typography>
      )}
      {formattedCLs.length === 1 && (
        <Link
          href={formattedCLs[0].url}
          target="_blank"
          rel="noopener noreferrer"
          underline="hover"
        >
          {formattedCLs[0].display}
        </Link>
      )}
      {formattedCLs.length > 1 && (
        <>
          <Link
            href={formattedCLs[0].url}
            target="_blank"
            rel="noopener noreferrer"
            underline="hover"
          >
            {formattedCLs[0].display}
          </Link>
          <ButtonBase
            onClick={handleCLPopoverOpen}
            aria-describedby="cl-popover-assoc-cls"
            sx={{
              ml: 0.5,
              textDecoration: 'underline',
              color: 'primary.main',
              cursor: 'pointer',
              typography: 'body2',
            }}
          >
            + {formattedCLs.length - 1} more
          </ButtonBase>
        </>
      )}
      {formattedCLs.length > 0 && formattedCLs[0].url && (
        <Link
          href={formattedCLs[0].url + clHistoryLinkSuffix}
          target="_blank"
          rel="noopener noreferrer"
          underline="hover"
          sx={{ ml: 0.5 }}
        >
          (View history)
        </Link>
      )}
      <CLsPopover
        anchorEl={clPopoverAnchorEl}
        open={clPopoverOpen}
        onClose={handleCLPopoverClose}
        id="cl-popover-assoc-cls"
      />
    </Box>
  );
}
