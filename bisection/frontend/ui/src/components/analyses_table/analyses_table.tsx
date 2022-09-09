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

import './analyses_table.css';

import { Link as RouterLink } from 'react-router-dom';

import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { NoDataMessageRow } from '../no_data_message_row/no_data_message_row';
import { Analysis } from '../../services/luci_bisection';
import { linkToBuilder } from '../../tools/link_constructors';
import {
  getFormattedDuration,
  getFormattedTimestamp,
} from '../../tools/timestamp_formatters';

interface AnalysisRowProps {
  analysis: Analysis;
}

interface AnalysisTableProps {
  analyses: Analysis[];
}

const AnalysisRow = ({ analysis }: AnalysisRowProps) => {
  const builderLink = linkToBuilder(analysis.builder);
  return (
    <TableRow key={analysis.analysisId} hover>
      <TableCell>
        <Link
          component={RouterLink}
          to={`/analysis/b/${analysis.firstFailedBbid}`}
        >
          {analysis.firstFailedBbid}
        </Link>
      </TableCell>
      <TableCell>{getFormattedTimestamp(analysis.createdTime)}</TableCell>
      <TableCell>{analysis.status}</TableCell>
      <TableCell>{analysis.buildFailureType}</TableCell>
      <TableCell>
        {getFormattedDuration(analysis.createdTime, analysis.endTime)}
      </TableCell>
      <TableCell>
        {analysis.builder && (
          <Link
            href={builderLink.url}
            target='_blank'
            rel='noreferrer'
            underline='always'
          >
            {builderLink.linkText}
          </Link>
        )}
      </TableCell>
      {/* TODO: add culprit CL information */}
    </TableRow>
  );
};

function getRows(analyses: Analysis[] | undefined) {
  if (!analyses || analyses.length == 0) {
    return <NoDataMessageRow message='No analyses to display' columns={6} />;
  } else {
    return analyses.map((analysis) => (
      <AnalysisRow key={analysis.analysisId} analysis={analysis} />
    ));
  }
}

export const AnalysesTable = ({ analyses }: AnalysisTableProps) => {
  // TODO: implement sorting & filtering for certain columns
  return (
    <TableContainer className='analyses-table-container' component={Paper}>
      <Table className='analyses-table' size='small'>
        <TableHead>
          <TableRow>
            <TableCell>Buildbucket ID</TableCell>
            <TableCell>Created time</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Failure type</TableCell>
            <TableCell>Duration</TableCell>
            <TableCell>Builder</TableCell>
            {/* TODO: add column for culprit once culprit information is available */}
          </TableRow>
        </TableHead>
        <TableBody>{getRows(analyses)}</TableBody>
      </Table>
    </TableContainer>
  );
};
