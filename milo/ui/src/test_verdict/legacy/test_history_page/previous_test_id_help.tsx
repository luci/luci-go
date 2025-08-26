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

import { HelpOutline } from '@mui/icons-material';
import { IconButton, Popover, Typography } from '@mui/material';
import { useState } from 'react';

export interface PreviousTestIdBannerProps {
  readonly previousTestId: string;
}

export function PreviousTestIDHelp({
  previousTestId,
}: PreviousTestIdBannerProps) {
  const [helpAnchorEl, setHelpAnchorEl] = useState<HTMLButtonElement | null>(
    null,
  );

  return (
    <>
      <IconButton
        aria-label="toggle help"
        edge="end"
        onClick={(e) => {
          setHelpAnchorEl(e.currentTarget);
        }}
        sx={{ p: 0, marginRight: 0 }}
      >
        <HelpOutline fontSize="small" />
      </IconButton>
      <Popover
        open={Boolean(helpAnchorEl)}
        anchorEl={helpAnchorEl}
        onClose={() => setHelpAnchorEl(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
      >
        <Typography sx={{ p: 2, maxWidth: '800px' }}>
          This test is migrating to a new structured test identifier to help us
          organise tests better. It may have additional history under its old
          ID:{' '}
          <code style={{ overflowWrap: 'break-word' }}>{previousTestId}</code>
        </Typography>
      </Popover>
    </>
  );
}
