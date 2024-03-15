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
import { Box, IconButton, TableCell, styled } from '@mui/material';
import { Fragment } from 'react';

import { ChangelistLink } from '@/gitiles/components/changelist_link';

import {
  useBuild,
  useDefaultExpanded,
  useSetDefaultExpanded,
  useSetRowExpanded,
} from '../context';

export function GerritChangesHeadCell() {
  const defaultExpanded = useDefaultExpanded();
  const setDefaultExpanded = useSetDefaultExpanded();

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
          onClick={() => setDefaultExpanded(!defaultExpanded)}
        >
          {defaultExpanded ? <ExpandMore /> : <ChevronRight />}
        </IconButton>
        <div css={{ lineHeight: '34px' }}>Changes</div>
      </div>
    </TableCell>
  );
}

const ChangesContainer = styled(Box)({
  width: '200px',
  lineHeight: '32px',
  '.BuildTableRow-expanded &': {
    whiteSpace: 'pre',
  },
  '.BuildTableRow-collapsed &': {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    '& > br': {
      display: 'none',
    },
  },
});

export function GerritChangesContentCell() {
  const setExpanded = useSetRowExpanded();
  const build = useBuild();

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
            onClick={() => setExpanded((expanded) => !expanded)}
            // Always render the button to DOM so we have a stable layout.
            // Hide it from users so it won't mislead users to think there
            // are more gerrit changes.
            disabled={changes.length <= 1}
            sx={{ visibility: changes.length > 1 ? '' : 'hidden' }}
          >
            <ExpandMore
              sx={{ '.BuildTableRow-collapsed &': { display: 'none' } }}
            />
            <ChevronRight
              sx={{ '.BuildTableRow-expanded &': { display: 'none' } }}
            />
          </IconButton>
        </div>
        <ChangesContainer>
          {changes.map((c, i) => (
            <Fragment key={c.change}>
              {i !== 0 && (
                <>
                  , <br />
                </>
              )}
              <ChangelistLink changelist={c} />
            </Fragment>
          ))}
        </ChangesContainer>
      </div>
    </TableCell>
  );
}
