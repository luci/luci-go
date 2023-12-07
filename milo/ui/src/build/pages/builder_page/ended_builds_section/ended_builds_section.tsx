// Copyright 2023 The LUCI Authors.
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
import { DateTimePicker } from '@mui/x-date-pickers';
import { DateTime } from 'luxon';
import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';

import { OutputBuild } from '@/build/types';
import { usePrpcQuery } from '@/common/hooks/prpc_query';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  BuildsClientImpl,
  SearchBuildsRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { EndedBuildTable } from './ended_build_table';
import {
  createdBeforeUpdater,
  getCreatedBefore,
  getPageSize,
  getPageToken,
  pageSizeUpdater,
  pageTokenUpdater,
} from './search_param_utils';

const FIELD_MASK = Object.freeze([
  'builds.*.status',
  'builds.*.id',
  'builds.*.number',
  'builds.*.create_time',
  'builds.*.end_time',
  'builds.*.start_time',
  'builds.*.output.gitiles_commit',
  'builds.*.input.gitiles_commit',
  'builds.*.input.gerrit_changes',
  'builds.*.summary_markdown',
]);

export interface EndedBuildsSectionProps {
  readonly builderId: BuilderID;
}

export function EndedBuildsSection({ builderId }: EndedBuildsSectionProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const pageSize = getPageSize(searchParams);
  const pageToken = getPageToken(searchParams);
  const createdBefore = getCreatedBefore(searchParams);

  // There could be a lot of prev pages. Do not keep those tokens in the URL.
  const [prevPageTokens, setPrevPageTokens] = useState(() => {
    // If there's a page token when the component is FIRST INITIALIZED, allow
    // users to go back to the first page by inserting a blank page token.
    return pageToken ? [''] : [];
  });

  const headingRef = useRef<HTMLHeadingElement>(null);

  const { data, error, isError, isLoading, isPreviousData } = usePrpcQuery({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
    method: 'SearchBuilds',
    request: SearchBuildsRequest.fromPartial({
      predicate: {
        builder: builderId,
        includeExperimental: true,
        status: Status.ENDED_MASK,
        createTime: {
          endTime: createdBefore?.toISO(),
        },
      },
      pageSize,
      pageToken,
      fields: FIELD_MASK,
    }),
    options: { keepPreviousData: true },
  });

  if (isError) {
    throw error;
  }

  const prevPageToken = prevPageTokens.length
    ? prevPageTokens[prevPageTokens.length - 1]
    : null;
  const nextPageToken =
    !isPreviousData && data?.nextPageToken ? data?.nextPageToken : null;
  const builds = (data?.builds || []) as readonly OutputBuild[];

  return (
    <>
      <h3 ref={headingRef}>Ended Builds</h3>
      <Box>
        <DateTimePicker
          label="Created Before"
          format={SHORT_TIME_FORMAT}
          value={createdBefore}
          // Buildbucket only retain builds for ~1.5 years. No point going
          // further than that.
          minDate={DateTime.now().minus({ year: 1, months: 6 })}
          disableFuture
          slotProps={{
            actionBar: {
              actions: ['today', 'clear', 'cancel', 'accept'],
            },
            textField: {
              size: 'small',
            },
          }}
          onAccept={(t) => {
            setPrevPageTokens([]);
            setSearchParams(createdBeforeUpdater(t));
            setSearchParams(pageTokenUpdater(''));
            headingRef.current?.scrollIntoView();
          }}
        />
      </Box>
      <EndedBuildTable
        endedBuilds={builds}
        isLoading={isLoading || isPreviousData}
      />
      <Box sx={{ mt: '5px' }}>
        Page Size:{' '}
        <ToggleButtonGroup exclusive value={pageSize} size="small">
          {[25, 50, 100, 200].map((s) => (
            <ToggleButton
              key={s}
              component={Link}
              to={`?${pageSizeUpdater(s)(searchParams)}`}
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
            headingRef.current?.scrollIntoView();
          }}
        >
          Previous Page
        </Button>
        <Button
          disabled={nextPageToken === null}
          component={Link}
          to={`?${pageTokenUpdater(nextPageToken || '')(searchParams)}`}
          onClick={(e) => {
            if (e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
              return;
            }

            setPrevPageTokens([...prevPageTokens, pageToken]);
            headingRef.current?.scrollIntoView();
          }}
        >
          Next Page
        </Button>
      </Box>
    </>
  );
}
