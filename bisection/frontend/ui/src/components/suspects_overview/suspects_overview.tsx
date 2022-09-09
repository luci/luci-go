// Copyright 2022 The LUCI Authors.
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


import './suspects_overview.css';

import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { NoDataMessageRow } from '../no_data_message_row/no_data_message_row';
import { PrimeSuspect } from '../../services/luci_bisection';

interface Props {
  suspects: PrimeSuspect[];
}

export const SuspectsOverview = ({ suspects }: Props) => {
  return (
    <TableContainer className='suspects-overview' component={Paper}>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>Suspect CL</TableCell>
            <TableCell>Source analysis</TableCell>
            <TableCell>Culprit status</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {suspects.map((suspect) => (
            <TableRow key={suspect.cl.commitID}>
              <TableCell>
                <Link
                  href={suspect.cl.reviewURL}
                  target='_blank'
                  rel='noreferrer'
                  underline='always'
                >
                  {suspect.cl.title}
                </Link>
              </TableCell>
              <TableCell>{suspect.accuseSource}</TableCell>
              <TableCell>{suspect.culpritStatus}</TableCell>
            </TableRow>
          ))}
          {suspects.length === 0 && (
            <NoDataMessageRow message='No suspects to display' columns={3} />
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
