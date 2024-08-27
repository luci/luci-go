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

import { getBuilderURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { SearchBuildsRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { useBuildsClient } from './context';

const SEARCH_BUILD_FIELD_MASK = Object.freeze(['status']);

const Container = styled.div({
  display: 'grid',
  gridTemplateColumns: 'auto auto',
  height: '20px',
  gap: '5px',
  '& > .stats-badge': {
    fontSize: '10px',
    display: 'inline-block',
    width: '55px',
    height: '20px',
    lineHeight: '20px',
    textAlign: 'center',
    border: '1px solid black',
    borderRadius: '3px',
    textDecoration: 'none',
    color: 'var(--default-text-color)',
  },
  // TODO: extract the color pattern to a theme or a common CSS file.
  '& > .pending-cell': {
    backgroundColor: '#ccc',
  },
  '& > .running-cell': {
    backgroundColor: '#fd3',
  },
});

export interface BuilderStatsProps {
  readonly builder: BuilderID;
}

export function BuilderStats({ builder }: BuilderStatsProps) {
  const builderLink = getBuilderURLPath(builder);

  const client = useBuildsClient();
  const { data, isLoading, isError, error } = useQuery({
    ...client.Batch.query({
      requests: [
        {
          searchBuilds: SearchBuildsRequest.fromPartial({
            predicate: {
              builder,
              includeExperimental: true,
              status: Status.SCHEDULED,
            },
            mask: {
              fields: SEARCH_BUILD_FIELD_MASK,
            },
          }),
        },
        {
          searchBuilds: SearchBuildsRequest.fromPartial({
            predicate: {
              builder,
              includeExperimental: true,
              status: Status.STARTED,
            },
            mask: {
              fields: SEARCH_BUILD_FIELD_MASK,
            },
          }),
        },
      ],
    }),
    select: (res) => {
      return {
        pendingBuildCount: res.responses[0].searchBuilds?.builds.length,
        runningBuildCount: res.responses[1].searchBuilds?.builds.length,
      };
    },
  });
  if (isError) {
    throw error;
  }

  if (isLoading) {
    return (
      <Container>
        <CircularProgress size={20} />
      </Container>
    );
  }

  return (
    <Container>
      <Link
        component={RouterLink}
        to={builderLink}
        className="stats-badge pending-cell"
      >
        {data?.pendingBuildCount || 0} pending
      </Link>
      <Link
        component={RouterLink}
        to={builderLink}
        className="stats-badge running-cell"
      >
        {data?.runningBuildCount || 0} running
      </Link>
    </Container>
  );
}
