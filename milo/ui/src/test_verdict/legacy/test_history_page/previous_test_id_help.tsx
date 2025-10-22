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
import Link from '@mui/material/Link';
import { useMemo, useState } from 'react';
import { Link as RouterLink } from 'react-router';

import { getTestHistoryURLWithSearchParam } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export interface PreviousTestIdBannerProps {
  readonly project: string;
  readonly previousTestId: string;
}

export function PreviousTestIDHelp({
  project,
  previousTestId,
}: PreviousTestIdBannerProps) {
  const [helpAnchorEl, setHelpAnchorEl] = useState<HTMLButtonElement | null>(
    null,
  );

  const [searchParams, _] = useSyncedSearchParams();

  const queryParam = searchParams.get('q') || '';
  const previousTestIdHistoryURL = useMemo(() => {
    return getTestHistoryURLWithSearchParam(
      project,
      previousTestId,
      queryParam,
      // Redirect. Set this to avoid the page for the old test ID auto-redirecting back
      // to the page for the new test ID.
      true,
    );
  }, [project, previousTestId, queryParam]);

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
          organise tests better. It may have some history under its old ID:{' '}
          <code style={{ overflowWrap: 'break-word' }}>{previousTestId}</code>
          <Link
            component={RouterLink}
            to={previousTestIdHistoryURL}
            underline="always"
            sx={{ display: 'flex', alignItems: 'center' }}
          >
            View history for previous ID only
          </Link>
        </Typography>
      </Popover>
    </>
  );
}
