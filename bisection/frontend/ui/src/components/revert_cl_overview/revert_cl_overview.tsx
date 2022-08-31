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


import Link from '@mui/material/Link';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';

import { RevertCL } from '../../services/luci_bisection';
import { PlainTable } from '../plain_table/plain_table';

interface Props {
  revertCL: RevertCL;
}

export const RevertCLOverview = ({ revertCL }: Props) => {
  return (
    <TableContainer>
      <PlainTable>
        <colgroup>
          <col style={{ width: '15%' }} />
          <col style={{ width: '85%' }} />
        </colgroup>
        <TableBody data-testid='change_list_overview_table_body'>
          <TableRow>
            <TableCell variant='head' colSpan={2}>
              <Link
                href={revertCL.cl.reviewURL}
                target='_blank'
                rel='noreferrer'
                underline='always'
              >
                {revertCL.cl.title}
              </Link>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Status</TableCell>
            <TableCell>{revertCL.status}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Submitted time</TableCell>
            <TableCell>{revertCL.submitTime}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Commit position</TableCell>
            <TableCell>{revertCL.commitPosition}</TableCell>
          </TableRow>
        </TableBody>
      </PlainTable>
    </TableContainer>
  );
};
