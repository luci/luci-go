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
import { Fragment } from 'react';

import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { TestHistoryService } from '@/common/services/luci_analysis';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';

interface Props {
  searchQuery: string;
  project: string;
}

export function TestList({ project, searchQuery }: Props) {
  const { data, isError, error, isLoading, fetchNextPage, hasNextPage } =
    useInfinitePrpcQuery({
      host: SETTINGS.luciAnalysis.host,
      Service: TestHistoryService,
      method: 'queryTests',
      request: {
        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
        project: project,
        testIdSubstring: searchQuery,
      },
      options: {
        enabled: searchQuery !== '',
      },
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
