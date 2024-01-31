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

import { TableVirtuoso } from 'react-virtuoso';

import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

import { BuilderRow } from './builder_row';
import { BuilderTableContextProvider } from './context';

export interface BuilderTableProps {
  readonly builders: readonly BuilderID[];
  /**
   * Number of recent builds per builder row.
   */
  readonly numOfBuilds: number;
}

export function BuilderTable({ builders, numOfBuilds }: BuilderTableProps) {
  return (
    <BuilderTableContextProvider numOfBuilds={numOfBuilds}>
      <TableVirtuoso
        useWindowScroll
        components={{
          TableRow: BuilderRow,
        }}
        data={builders}
        fixedItemHeight={40}
        css={{
          '& table': {
            width: '100%',
          },
        }}
      />
    </BuilderTableContextProvider>
  );
}
