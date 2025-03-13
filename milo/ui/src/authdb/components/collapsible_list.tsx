// Copyright 2025 The LUCI Authors.
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

import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import IconButton from '@mui/material/IconButton';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useState } from 'react';

import { GroupLink } from '@/authdb/components/group_link';

interface CollapsibleListProps {
  items: string[];
  // Determines whether to use GroupLink or not.
  renderAsGroupLinks: boolean;
  title: string;
}

const expansionThreshold = 10;

export function CollapsibleList({
  items,
  renderAsGroupLinks,
  title,
}: CollapsibleListProps) {
  const [expanded, setExpanded] = useState<boolean>(true);
  const expandable = items.length > expansionThreshold;

  const titleRow = (
    <TableRow>
      <TableCell>
        <Typography variant="h6">{title}</Typography>
        {expandable && (
          <IconButton
            sx={{ pb: 0, pt: 0 }}
            onClick={() => {
              setExpanded(!expanded);
            }}
          >
            {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        )}
      </TableCell>
    </TableRow>
  );

  if (items.length === 0) {
    return (
      <>
        {titleRow}
        <Typography
          variant="body2"
          sx={{ fontStyle: 'italic', pl: '20px', pb: '16px', color: 'grey' }}
        >
          None
        </Typography>
      </>
    );
  }

  return (
    <>
      {titleRow}
      {(expanded || !expandable) && (
        <>
          <TableRow>
            <TableCell sx={{ pt: 0, pb: '16px' }}>
              <ul>
                {items.map((item) => {
                  return (
                    <li key={item}>
                      {renderAsGroupLinks ? (
                        <GroupLink name={item} />
                      ) : (
                        <Typography variant="body2">{item}</Typography>
                      )}
                    </li>
                  );
                })}
              </ul>
            </TableCell>
          </TableRow>
        </>
      )}
    </>
  );
}
