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

import { Box, Link, Typography } from '@mui/material';
import React, { useCallback, useState } from 'react';

import { CLsPopover } from '@/test_investigation/components/test_info/cls_popver';

import { useFormattedCLs } from '../context';

const clHistoryLinkSuffix = '/+log';

interface AssociatedCLsSectionProps {
  expanded: boolean;
}

export function AssociatedCLsSection({ expanded }: AssociatedCLsSectionProps) {
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

  if (formattedCLs.length === 0) {
    return <></>;
  }

  return (
    <Box>
      <Typography
        variant="body2"
        color="text.secondary"
        component="div"
        sx={{ mb: expanded ? 1 : 2 }}
      >
        CLs
      </Typography>
      <Box
        sx={{
          display: 'flex',
          flexDirection: expanded ? 'row' : 'column',
          gap: '5px',
          alignItems: 'left',
          justifyContent: 'start',
        }}
      >
        <Link
          href={formattedCLs[0].url}
          target="_blank"
          rel="noopener noreferrer"
          underline="hover"
        >
          {formattedCLs[0].display}
        </Link>
        {formattedCLs.length > 1 && (
          <Link
            onClick={handleCLPopoverOpen}
            aria-describedby="cl-popover-assoc-cls"
            style={{ cursor: 'pointer' }}
          >
            + {formattedCLs.length - 1} more in same topic
          </Link>
        )}
        {formattedCLs[0].url && (
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
    </Box>
  );
}
