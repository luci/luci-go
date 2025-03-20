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
  numRedacted?: number;
}

export function CollapsibleList({
  items,
  renderAsGroupLinks,
  title,
  numRedacted = 0,
}: CollapsibleListProps) {
  const [expanded, setExpanded] = useState<boolean>(true);

  if (items.length === 0) {
    return (
      <>
        <Typography variant="h6">{title}</Typography>
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
      <TableRow
        onClick={() => {
          setExpanded(!expanded);
        }}
        style={{ cursor: 'pointer' }}
      >
        <TableCell>
          <Typography variant="h6">{title}</Typography>
          <IconButton sx={{ pb: 0, pt: 0 }}>
            {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        </TableCell>
      </TableRow>
      {expanded && (
        <>
          <TableRow data-testid="collapsible-list">
            <TableCell sx={{ pt: 0, pb: '16px' }}>
              <ul>
                {numRedacted > 0 && (
                  <Typography
                    variant="body2"
                    sx={{
                      fontStyle: 'italic',
                      color: 'grey',
                    }}
                  >
                    {numRedacted} members redacted.
                  </Typography>
                )}
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
