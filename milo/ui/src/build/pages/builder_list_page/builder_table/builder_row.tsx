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

import { getBuilderURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

const Row = styled('tr')({
  height: '40px',
  '& > td': {
    padding: '5px',
  },
  // All columns except the last one shrink to fit.
  '& > td:not(:last-of-type)': {
    width: '1px',
    whiteSpace: 'nowrap',
  },
});

export interface BuilderRowProps {
  readonly 'data-index': number;
  readonly item: BuilderID;
}

export function BuilderRow({
  'data-index': index,
  item: builder,
}: BuilderRowProps) {
  return (
    <Row
      css={{
        // Apply checkered style to the rows.
        background: index % 2 === 1 ? 'var(--block-background-color)' : '',
      }}
    >
      <td>
        <Link href={getBuilderURLPath(builder)}>
          {builder.project}/{builder.bucket}/{builder.builder}
        </Link>
      </td>
      <td>{/* TODO: render builder stats */}</td>
      <td>{/* TODO: render recent builds */}</td>
    </Row>
  );
}
