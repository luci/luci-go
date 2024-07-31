// Copyright 2024 The LUCI Authors.
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

import { Box, Button, ToggleButton, ToggleButtonGroup } from '@mui/material';
import { useState } from 'react';
import { Link } from 'react-router-dom';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  getPageSize,
  getPageToken,
  pageSizeUpdater,
  pageTokenUpdater,
} from './params_pager_utils';

export interface ParamsPagerProps {
  readonly nextPageToken: string;
  readonly pageSizes?: number[];
  readonly defaultPageSize?: number;
}

/**
 * ParamsPager shows a page size and a next/previous page buttons.
 * This assumes an AIP-158 style pagination API (i.e. pageSize and pageToken fields)
 * This component does not emit events for updates, but instead stores the current
 * page and pageSize in the URL.
 * For the previous page button to work correctly you need to take care to not remount
 * the component each time you navigate to a page.  E.g. loading spinners must not
 * replace this component, but must be lower in the component hierarchy.
 */
export function ParamsPager({
  nextPageToken,
  pageSizes = [25, 50, 100, 200],
  defaultPageSize = 50,
}: ParamsPagerProps) {
  const [searchParams, _] = useSyncedSearchParams();
  const pageSize = getPageSize(searchParams, defaultPageSize);
  const pageToken = getPageToken(searchParams);

  // There could be a lot of prev pages. Do not keep those tokens in the URL.
  const [prevPageTokens, setPrevPageTokens] = useState(() => {
    // If there's a page token when the component is FIRST INITIALIZED, allow
    // users to go back to the first page by inserting a blank page token.
    return pageToken ? [''] : [];
  });

  const prevPageToken = prevPageTokens.length
    ? prevPageTokens[prevPageTokens.length - 1]
    : null;
  return (
    <>
      <Box sx={{ mt: '5px' }}>
        Page Size:{' '}
        <ToggleButtonGroup exclusive value={pageSize} size="small">
          {pageSizes.map((s) => (
            <ToggleButton
              key={s}
              component={Link}
              to={`?${pageSizeUpdater(s, defaultPageSize)(searchParams)}`}
              value={s}
            >
              {s}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>{' '}
        <Button
          disabled={prevPageToken === null}
          component={Link}
          to={`?${pageTokenUpdater(prevPageToken || '')(searchParams)}`}
          onClick={(e) => {
            if (e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
              return;
            }
            setPrevPageTokens(prevPageTokens.slice(0, -1));
          }}
        >
          Previous Page
        </Button>
        <Button
          disabled={nextPageToken === ''}
          component={Link}
          to={`?${pageTokenUpdater(nextPageToken)(searchParams)}`}
          onClick={(e) => {
            if (e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
              return;
            }
            setPrevPageTokens([...prevPageTokens, pageToken]);
          }}
        >
          Next Page
        </Button>
      </Box>
    </>
  );
}
