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

import dayjs from 'dayjs';

import Chip from '@mui/material/Chip';
import Typography from '@mui/material/Typography';
import TableRow from '@mui/material/TableRow';
import TableCell from '@mui/material/TableCell';

import { Problem } from '../problems';
import { PriorityChip } from '../priority_chip/priority_chip';
import { StatusChip } from '../status_chip/status_chip';

interface Props {
  problem: Problem;
  openProblemDialog: (p: Problem) => void;
}

export const ProblemRow = ({ problem, openProblemDialog }: Props) => {
  const handleOpenClicked = () => {
    openProblemDialog(problem);
  };
  return <TableRow key={problem.policy.id} data-testid={'policy-row-' + problem.policy.id}>
    <TableCell sx={{ fontSize: '1rem', paddingLeft: '0px', textDecoration: problem.state.isActive ? 'none' : 'line-through' }}>{problem.policy.humanReadableName}</TableCell>
    <TableCell sx={{ fontSize: '1rem' }}><StatusChip isActive={problem.state.isActive} /></TableCell>
    <TableCell sx={{ fontSize: '1rem' }}>{ problem.state.isActive ? <PriorityChip priority={problem.policy.priority} /> : '-' }</TableCell>
    <TableCell sx={{ fontSize: '1rem' }}>{ problem.state.isActive ? dayjs.utc(problem.state.lastActivationTime).local().fromNow() : '-' }</TableCell>
    <TableCell>
      <Chip
        aria-label="View details of problems"
        variant="outlined"
        color="default"
        onClick={handleOpenClicked}
        label={
          <Typography variant="button">more info</Typography>
        }
        sx={{ borderRadius: 1 }} />
    </TableCell>
  </TableRow>;
};
