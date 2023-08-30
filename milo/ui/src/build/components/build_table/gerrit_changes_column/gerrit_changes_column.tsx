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

import { ChevronRight, ExpandMore } from '@mui/icons-material';
import { IconButton, TableCell } from '@mui/material';
import { computed } from 'mobx';
import { observer } from 'mobx-react-lite';
import { Fragment, useMemo } from 'react';

import { GerritClLink } from '@/common/components/gerrit_cl_link';

import { useRowState, useTableState } from '../context';

export const GerritChangesHeadCell = observer(() => {
  const tableState = useTableState();

  return (
    <TableCell width="1px">
      <div
        css={{
          display: 'grid',
          gridTemplateColumns: '34px 1fr',
        }}
      >
        <IconButton
          aria-label="toggle-all-rows"
          size="small"
          onClick={() => tableState.toggleAll(!tableState.defaultExpanded)}
        >
          {tableState.defaultExpanded ? <ExpandMore /> : <ChevronRight />}
        </IconButton>
        <div css={{ lineHeight: '34px' }}>Changes</div>
      </div>
    </TableCell>
  );
});

export const GerritChangesContentCell = observer(() => {
  const tableState = useTableState();
  const build = useRowState();

  const expandedObservable = useMemo(
    () =>
      computed(() => {
        // When there's no gerrit changes, always treat the cell as expanded so
        // the component doesn't need to be updated when the default expansion
        // state is updated.
        return (
          !build.input?.gerritChanges?.length || tableState.isExpanded(build.id)
        );
      }),
    [build, tableState],
  );
  const expanded = expandedObservable.get();

  const changes = build.input?.gerritChanges || [];

  return (
    <TableCell>
      <div
        css={{
          display: 'grid',
          gridTemplateColumns: '34px 1fr',
        }}
      >
        <div>
          <IconButton
            aria-label="toggle-row"
            size="small"
            onClick={() => tableState.toggle(build.id, !expanded)}
            // Always render the button to DOM so we have a stable layout.
            // Hide it from users so it won't mislead users to think there
            // are more gerrit changes.
            disabled={changes.length <= 1}
            sx={{ visibility: changes.length > 1 ? '' : 'hidden' }}
          >
            {expanded ? <ExpandMore /> : <ChevronRight />}
          </IconButton>
        </div>
        <div
          css={{
            width: '200px',
            lineHeight: '32px',
            ...(expanded
              ? { whiteSpace: 'pre' }
              : {
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  '& > br': {
                    display: 'none',
                  },
                }),
          }}
        >
          {changes.map((c, i) => (
            <Fragment key={c.change}>
              {i !== 0 && (
                <>
                  , <br />
                </>
              )}
              <GerritClLink cl={c} />
            </Fragment>
          ))}
        </div>
      </div>
    </TableCell>
  );
});
