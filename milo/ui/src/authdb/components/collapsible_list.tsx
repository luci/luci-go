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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Icon from '@mui/material/Icon';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useEffect, useState } from 'react';

import { GroupLink } from '@/authdb/components/group_link';
import { AuthLookupLink } from '@/authdb/components/lookup_link';

interface CollapsibleListProps {
  items: string[];
  title: string;
  globallyExpanded?: boolean;
  variant?: 'text' | 'group-link' | 'principal-link';
  numRedacted?: number;
}

export function CollapsibleList({
  items,
  title,
  globallyExpanded = true,
  variant = 'text',
  numRedacted = 0,
}: CollapsibleListProps) {
  const [expanded, setExpanded] = useState<boolean>(true);

  useEffect(() => {
    setExpanded(globallyExpanded);
  }, [globallyExpanded]);

  const count = items.length + numRedacted;

  return (
    <>
      <TableRow
        onClick={() => {
          setExpanded(!expanded);
        }}
        style={{ cursor: 'pointer' }}
      >
        <TableCell>
          <Icon sx={{ pb: 0, pt: 0 }}>
            {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
          </Icon>
          <Typography variant="h6">
            {title} ({count})
          </Typography>
        </TableCell>
      </TableRow>
      {expanded && (
        <>
          <TableRow data-testid="collapsible-list">
            <TableCell sx={{ pt: 0, pb: '16px' }}>
              <ul>
                {count === 0 && (
                  <li>
                    <Typography
                      component="div"
                      variant="body2"
                      color="textSecondary"
                      sx={{
                        fontStyle: 'italic',
                      }}
                    >
                      None
                    </Typography>
                  </li>
                )}
                {numRedacted > 0 && (
                  <li>
                    <Typography
                      component="div"
                      variant="body2"
                      color="textSecondary"
                      sx={{
                        fontStyle: 'italic',
                      }}
                    >
                      {numRedacted} members redacted.
                    </Typography>
                  </li>
                )}
                {variant === 'group-link' &&
                  items.map((item) => (
                    <li key={item}>
                      <GroupLink name={item}></GroupLink>
                    </li>
                  ))}
                {variant === 'principal-link' &&
                  items.map((item) => (
                    <li key={item}>
                      <AuthLookupLink principal={item} />
                    </li>
                  ))}
                {variant === 'text' &&
                  items.map((item) => (
                    <li key={item}>
                      <Typography component="div" variant="body2">
                        {item}
                      </Typography>
                    </li>
                  ))}
              </ul>
            </TableCell>
          </TableRow>
        </>
      )}
    </>
  );
}
