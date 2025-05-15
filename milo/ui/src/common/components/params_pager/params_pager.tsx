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
import { Link } from 'react-router';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  PagerContext,
  getPageSize,
  getPrevPageToken,
  pageSizeUpdater,
  pageTokenUpdater,
} from './context';

export interface ParamsPagerProps {
  /**
   * The context to store the pager's state in. Can only be obtained by calling
   * `usePagerContext`.
   *
   * Multiple `<ParamPagers />` with the same `pagerCtx` will share the same
   * list of previous page tokens. This allows you to place multiple pagers
   * (e.g. one at the top and one at the bottom).
   */
  readonly pagerCtx: PagerContext;
  /**
   * Empty `nextPageToken` means there's no next page.
   */
  readonly nextPageToken: string;
}

/**
 * ParamsPager shows a page size and a next/previous page buttons.
 *
 * This assumes an AIP-158 style pagination API (i.e. pageSize and pageToken
 * fields). It stores the current page and pageSize in the URL.
 *
 * Note: you should use `emptyPageTokenUpdater` to discard page tokens when the
 * page token context changes and the page tokens are no longer valid. See
 * https://google.aip.dev/158 for details.
 */
export function ParamsPager({ pagerCtx, nextPageToken }: ParamsPagerProps) {
  const [searchParams] = useSyncedSearchParams();
  const pageSize = getPageSize(pagerCtx, searchParams);

  const prevPageToken = getPrevPageToken(pagerCtx);

  return (
    <>
      <Box sx={{ mt: '5px' }}>
        Page Size:{' '}
        <ToggleButtonGroup exclusive value={pageSize} size="small">
          {pagerCtx.options.pageSizeOptions.map((s) => (
            <ToggleButton
              key={s}
              component={Link}
              to={`?${pageSizeUpdater(pagerCtx, s)(searchParams)}`}
              value={s}
              onClick={(e) => {
                if (e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
                  return;
                }
                return pagerCtx.options.onPageSizeUpdated?.();
              }}
            >
              {s}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>{' '}
        <Button
          disabled={prevPageToken === null}
          component={Link}
          to={`?${pageTokenUpdater(pagerCtx, prevPageToken || '')(searchParams)}`}
          onClick={(e) => {
            if (e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
              return;
            }
            pagerCtx.options.onPrevPage?.();
          }}
        >
          Previous Page
        </Button>
        <Button
          disabled={nextPageToken === ''}
          component={Link}
          to={`?${pageTokenUpdater(pagerCtx, nextPageToken)(searchParams)}`}
          onClick={(e) => {
            if (e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
              return;
            }
            pagerCtx.options.onNextPage?.();
          }}
        >
          Next Page
        </Button>
      </Box>
    </>
  );
}
