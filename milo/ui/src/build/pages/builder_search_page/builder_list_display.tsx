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

import { Link, Typography, styled } from '@mui/material';
import { memo } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { GroupedVirtuoso } from 'react-virtuoso';

import { getBuilderURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

const ItemContainer = styled('div')({
  height: '20px',
  marginLeft: '20px',
});

const GroupContainer = styled('div')({
  height: '30px',
  padding: '15px 0',
});

export interface BuilderListDisplayProps {
  readonly groupedBuilders: {
    readonly [x: string]: readonly BuilderID[];
  };
}

export const BuilderListDisplay = memo(function BuilderListDisplay({
  groupedBuilders,
}: BuilderListDisplayProps) {
  const groups = Object.entries(groupedBuilders);
  const groupCounts = groups.map(([_, builders]) => builders.length);
  const flattenedBuilders = groups.flatMap(([_, builders]) => builders);
  return (
    <GroupedVirtuoso
      useWindowScroll
      components={{
        Item: ItemContainer,
        Group: GroupContainer,
      }}
      groupCounts={groupCounts}
      groupContent={(index) => {
        return <Typography variant="h6">{groups[index][0]}</Typography>;
      }}
      itemContent={(index) => {
        const builder = flattenedBuilders[index];
        return (
          <Link
            component={RouterLink}
            to={getBuilderURLPath(builder)}
            data-testid="builder-data"
          >
            {builder.builder}
          </Link>
        );
      }}
    />
  );
});
