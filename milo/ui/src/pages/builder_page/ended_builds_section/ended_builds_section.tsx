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

import { Box, Button, CircularProgress, Input } from '@mui/material';
import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';

import { BuilderID, BuildStatusMask } from '../../../services/buildbucket';
import { useBuilds } from '../utils';
import { EndedBuildsTable } from './ended_builds_table';

const DEFAULT_PAGE_SIZE = 25;
const FIELD_MASK =
  'builds.*.status,builds.*.id,builds.*.number,builds.*.createTime,builds.*.endTime,builds.*.startTime,' +
  'builds.*.output.gitilesCommit,builds.*.input.gitilesCommit,builds.*.summaryMarkdown';

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

  // There could be a lot of prev pages. Do not keep those tokens in the URL.
  const [prevPageTokens, setPrevPageTokens] = useState(() => {
    // If there's a page token when the component is FIRST INITIALIZED, allow
    // users to go back to the first page by inserting a blank page token.
    return currentPageToken ? [''] : [];
  });

  const { data, error, isError, isLoading } = useBuilds(
    {
      predicate: { builder: builderId, includeExperimental: true, status: BuildStatusMask.EndedMask },
      pageSize,
      pageToken: currentPageToken,
      fields: FIELD_MASK,
    },
    { keepPreviousData: true }
  );

  if (isError) {
    throw error;
  }

  return (
    <>
      <h3>Ended Builds</h3>
      {isLoading ? (
        <CircularProgress />
      ) : (
        <>
          <EndedBuildsTable endedBuilds={data.builds || []} />
          <Box>
            Page Size:{' '}
            <Input
              type="number"
              inputProps={{ min: 25, max: 100 }}
              sx={{ width: 40 }}
              value={pageSize}
              onChange={(e) => setPageSize(Number(e.target.value))}
            />
            <Button
              disabled={!prevPageTokens.length}
              onClick={() => {
                const newPrevPageTokens = prevPageTokens.slice();
                setCurrentPageToken(newPrevPageTokens.pop()!);
                setPrevPageTokens(newPrevPageTokens);
              }}
            >
              Previous Page
            </Button>
            <Button
              disabled={!data.nextPageToken}
              onClick={() => {
                setCurrentPageToken(data.nextPageToken!);
                setPrevPageTokens([...prevPageTokens, currentPageToken]);
              }}
            >
              Next Page
            </Button>
          </Box>
        </>
      )}
    </>
  );
}
