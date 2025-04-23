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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Grid from '@mui/material/Grid2';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useState } from 'react';

import { useResult } from '../context';

export function ResultTags() {
  const [expanded, setExpanded] = useState(false);
  const tags = useResult().tags;
  function handleExpand(isExpanded: boolean) {
    setExpanded(isExpanded);
  }
  return (
    <Accordion
      variant="outlined"
      disableGutters
      expanded={expanded}
      onChange={() => handleExpand(!expanded)}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography sx={{ mr: 1 }}>Tags</Typography>
        <Typography
          sx={{
            display: expanded ? 'none' : 'inline-block',
            color: 'text.secondary',
            maxWidth: '700px',
            overflow: 'hidden',
            whiteSpace: 'nowrap',
            textOverflow: 'ellipsis',
          }}
        >
          {tags.map(
            (entry, i) => `${i > 0 ? ', ' : ''} ${entry.key} = ${entry.value}`,
          )}
        </Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Grid container>
          <Grid size={{ xs: 2 }}>
            <Table size="small">
              <TableBody>
                {tags.map((entry, i) => (
                  <TableRow key={entry.key + '/' + i}>
                    <TableCell>{entry.key}</TableCell>
                    <TableCell>{entry.value}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Grid>
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
}
