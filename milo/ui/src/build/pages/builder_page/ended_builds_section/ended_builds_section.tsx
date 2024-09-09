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

import { Box } from '@mui/material';
import { DateTimePicker } from '@mui/x-date-pickers';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';
import { useRef } from 'react';

import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { OutputBuild } from '@/build/types';
import {
  ParamsPager,
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { SearchBuildsRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { EndedBuildTable } from './ended_build_table';
import { createdBeforeUpdater, getCreatedBefore } from './search_param_utils';

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
  const headingRef = useRef<HTMLHeadingElement>(null);
  const pagerCtx = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 25,
    onPrevPage: () => headingRef.current?.scrollIntoView(),
    onNextPage: () => headingRef.current?.scrollIntoView(),
  });
  const pageSize = getPageSize(pagerCtx, searchParams);
  const pageToken = getPageToken(pagerCtx, searchParams);
  const createdBefore = getCreatedBefore(searchParams);

  const client = useBuildsClient();
  const req = SearchBuildsRequest.fromPartial({
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
  });
  const { data, error, isError, isLoading, isPreviousData } = useQuery({
    ...client.SearchBuilds.query(req),
    keepPreviousData: true,
  });

  if (isError) {
    throw error;
  }

  const nextPageToken = isPreviousData ? '' : data?.nextPageToken || '';
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
            setSearchParams(createdBeforeUpdater(t));
            setSearchParams(emptyPageTokenUpdater(pagerCtx));
            headingRef.current?.scrollIntoView();
          }}
        />
      </Box>
      <EndedBuildTable
        endedBuilds={builds}
        isLoading={isLoading || isPreviousData}
      />
      <Box sx={{ mt: '5px' }}>
        <ParamsPager pagerCtx={pagerCtx} nextPageToken={nextPageToken} />
      </Box>
    </>
  );
}
