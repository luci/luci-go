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
import { useSearchParams } from 'react-router-dom';

import { usePrpcQuery } from '@/common/hooks/use_prpc_query';
import {
  BuilderID,
  BuildsService,
  BuildStatusMask,
} from '@/common/services/buildbucket';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';

import { EndedBuildTable } from './ended_build_table';

const DEFAULT_PAGE_SIZE = 25;
const FIELD_MASK =
  'builds.*.status,builds.*.id,builds.*.number,builds.*.createTime,builds.*.endTime,builds.*.startTime,' +
  'builds.*.output.gitilesCommit,builds.*.input.gitilesCommit,builds.*.input.gerritChanges,builds.*.summaryMarkdown';

export interface EndedBuildsSectionProps {
  readonly builderId: BuilderID;
}

export function EndedBuildsSection({ builderId }: EndedBuildsSectionProps) {
  const [searchParam, setSearchParams] = useSearchParams();

  const pageSize = Number(searchParam.get('limit')) || DEFAULT_PAGE_SIZE;
  const setPageSize = (newPageSize: number) => {
    if (newPageSize === DEFAULT_PAGE_SIZE) {
      searchParam.delete('limit');
    } else {
      searchParam.set('limit', String(newPageSize));
    }
    setSearchParams(searchParam);
  };

  const currentPageToken = searchParam.get('cursor') || '';
  const setCurrentPageToken = (newPageToken: string) => {
    if (!newPageToken) {
      searchParam.delete('cursor');
    } else {
      searchParam.set('cursor', newPageToken);
    }
    setSearchParams(searchParam);
  };

  const createdBeforeTs = searchParam.get('createdBefore');
  const createdBefore = createdBeforeTs
    ? DateTime.fromSeconds(Number(createdBeforeTs))
    : null;
  const setCreatedBefore = (newCreatedBefore: DateTime | null) => {
    if (!newCreatedBefore) {
      searchParam.delete('createdBefore');
    } else {
      searchParam.set('createdBefore', String(newCreatedBefore.toSeconds()));
    }
    setSearchParams(searchParam);
  };

  // There could be a lot of prev pages. Do not keep those tokens in the URL.
  const [prevPageTokens, setPrevPageTokens] = useState(() => {
    // If there's a page token when the component is FIRST INITIALIZED, allow
    // users to go back to the first page by inserting a blank page token.
    return currentPageToken ? [''] : [];
  });

  const headingRef = useRef<HTMLHeadingElement>(null);

  const { data, error, isError, isLoading, isPreviousData } = usePrpcQuery({
    host: SETTINGS.buildbucket.host,
    Service: BuildsService,
    method: 'searchBuilds',
    request: {
      predicate: {
        builder: builderId,
        includeExperimental: true,
        status: BuildStatusMask.EndedMask,
        createTime: {
          endTime: createdBefore?.toISO(),
        },
      },
      pageSize,
      pageToken: currentPageToken,
      fields: FIELD_MASK,
    },
    options: { keepPreviousData: true },
  });

  if (isError) {
    throw error;
  }

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
            setCurrentPageToken('');
            setCreatedBefore(t);
            headingRef.current?.scrollIntoView();
          }}
        />
      </Box>
      <EndedBuildTable
        endedBuilds={data?.builds || []}
        isLoading={isLoading || isPreviousData}
      />
      <Box sx={{ mt: '5px' }}>
        Page Size:{' '}
        <ToggleButtonGroup
          exclusive
          value={pageSize}
          onChange={(_e, newValue: number) => setPageSize(newValue)}
          size="small"
        >
          <ToggleButton value={25}>25</ToggleButton>
          <ToggleButton value={50}>50</ToggleButton>
          <ToggleButton value={100}>100</ToggleButton>
          <ToggleButton value={200}>200</ToggleButton>
        </ToggleButtonGroup>{' '}
        <Button
          disabled={!prevPageTokens.length}
          onClick={() => {
            const newPrevPageTokens = prevPageTokens.slice();
            // The button is disabled when `newPrevPageTokens` is empty.
            // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
            setCurrentPageToken(newPrevPageTokens.pop()!);
            setPrevPageTokens(newPrevPageTokens);
            headingRef.current?.scrollIntoView();
          }}
        >
          Previous Page
        </Button>
        <Button
          disabled={isPreviousData || !data?.nextPageToken}
          onClick={() => {
            // The button is disabled when `nextPageToken` is empty.
            // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
            setCurrentPageToken(data!.nextPageToken!);
            setPrevPageTokens([...prevPageTokens, currentPageToken]);
            headingRef.current?.scrollIntoView();
          }}
        >
          Next Page
        </Button>
      </Box>
    </>
  );
}
