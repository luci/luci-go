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

import styled from '@emotion/styled';
import { CircularProgress, Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { Link as RouterLink } from 'react-router-dom';

import { BUILD_STATUS_CLASS_MAP } from '@/build/constants';
import { SpecifiedStatus } from '@/build/types';
import { getBuildURLPathFromBuildId } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { SearchBuildsRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { useBuildsClient, useNumOfBuilds } from './hooks';

const SEARCH_BUILD_FIELD_MASK = Object.freeze(['status', 'id']);

const Container = styled.div({
  display: 'grid',
  gridTemplateRows: '20px',
  width: '100%',
  '& .cell': {
    display: 'inline-block',
    visibility: 'hidden',
    height: '20px',
    lineHeight: '20px',
    margin: '0',
    textAlign: 'center',
    border: '1px solid black',
    borderRadius: '3px',
  },
  '& .cell.build': {
    visibility: 'visible',
    textDecoration: 'none',
    color: 'var(--default-text-color)',
  },

  '& .cell:not(:first-of-type)': {
    borderLeft: '0px',
  },
  '& .cell:first-of-type': {
    padding: '0 10px',
  },
  '& .cell:first-of-type:after': {
    content: '"latest"',
  },
  '& > .success-cell': {
    backgroundColor: '#8d4',
  },
  '& > .failure-cell': {
    backgroundColor: '#e88',
  },
  '& > .infra-failure-cell': {
    backgroundColor: '#c6c',
  },
  '& > .canceled-cell': {
    backgroundColor: '#8ef',
  },
});

export interface RecentBuildsProps {
  readonly builder: BuilderID;
}

export function RecentBuilds({ builder }: RecentBuildsProps) {
  const numOfBuilds = useNumOfBuilds();
  const client = useBuildsClient();
  const { data, isLoading, error, isError } = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        predicate: {
          builder,
          includeExperimental: true,
          status: Status.ENDED_MASK,
        },
        mask: {
          fields: SEARCH_BUILD_FIELD_MASK,
        },
        pageSize: numOfBuilds,
      }),
    ),
  );
  if (isError) {
    throw error;
  }

  if (isLoading) {
    return <CircularProgress size={20} />;
  }

  return (
    <Container
      css={{
        // Ensure the first cell has enough space to render the label.
        gridTemplateColumns: `minmax(auto, 1fr) repeat(${
          numOfBuilds - 1
        }, 1fr)`,
      }}
    >
      {data.builds.map((build) => {
        const statusClass =
          BUILD_STATUS_CLASS_MAP[build.status as SpecifiedStatus];
        return (
          <Link
            component={RouterLink}
            key={build.id}
            className={`cell build ${statusClass}-cell`}
            to={getBuildURLPathFromBuildId(build.id)}
            target="_blank"
            rel="noreferrer"
          />
        );
      })}
    </Container>
  );
}
