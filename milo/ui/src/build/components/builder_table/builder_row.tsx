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

import { Link } from '@mui/material';
import { styled } from '@mui/system';
import { Link as RouterLink } from 'react-router';

import { getBuilderURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

import { BuilderStats } from './builder_stats';
import { RecentBuilds } from './recent_builds';

const Row = styled('tr')({
  height: '40px',
  '& > td': {
    padding: '5px',
    borderBottom: '1px solid rgb(224, 224, 224)',
  },
  // All columns except the last one shrink to fit.
  '& > td:not(:last-of-type)': {
    width: '1px',
    whiteSpace: 'nowrap',
  },
});

export interface BuilderRowProps {
  readonly item: BuilderID;
}

export function BuilderRow({ item: builder }: BuilderRowProps) {
  return (
    <Row>
      <td>
        <Link component={RouterLink} to={getBuilderURLPath(builder)}>
          {builder.project}/{builder.bucket}/{builder.builder}
        </Link>
      </td>
      <td>
        <BuilderStats builder={builder} />
      </td>
      <td>
        <RecentBuilds builder={builder} />
      </td>
    </Row>
  );
}
