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

import { Link, Typography } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { Fragment } from 'react';

import { useTestHistoryClient } from '@/analysis/hooks/prpc_clients';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { QueryTestsRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';

interface Props {
  searchQuery: string;
  project: string;
}

export function TestList({ project, searchQuery }: Props) {
  const client = useTestHistoryClient();
  const { data, isError, error, isLoading, fetchNextPage, hasNextPage } =
    useInfiniteQuery({
      ...client.QueryTests.queryPaged(
        QueryTestsRequest.fromPartial({
          project: project,
          testIdSubstring: searchQuery,
        }),
      ),
      enabled: searchQuery !== '',
    });

  if (isError) {
    throw error;
  }

  return (
    <>
      <ul>
        {data?.pages.map((p, i) => (
          <Fragment key={i}>
            {p.testIds?.map((testId) => (
              <li key={testId}>
                <Link
                  href={`/ui/test/${encodeURIComponent(
                    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
                    project!,
                  )}/${encodeURIComponent(testId)}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {testId}
                </Link>
              </li>
            ))}
          </Fragment>
        ))}
      </ul>
      {isLoading && searchQuery !== '' ? (
        <Typography component="span">
          Loading
          <DotSpinner />
        </Typography>
      ) : (
        hasNextPage && (
          <Typography
            component="span"
            className="active-text"
            onClick={() => fetchNextPage()}
          >
            [load more]
          </Typography>
        )
      )}
    </>
  );
}
