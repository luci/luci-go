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

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { Link as RouterLink } from 'react-router-dom';

import { linkToBuilder } from '@/bisection/tools/link_constructors';
import {
  getFormattedDuration,
  getFormattedTimestamp,
} from '@/bisection/tools/timestamp_formatters';
import { Analysis } from '@/common/services/luci_bisection';

import { CulpritsTableCell } from './culprit_table_cell';

interface AnalysisTableProps {
  analysis: Analysis;
}

export function AnalysisTableRow({ analysis }: AnalysisTableProps) {
  const builderLink = analysis.builder ? linkToBuilder(analysis.builder) : null;

  return (
    <TableRow hover data-testid="analysis_table_row">
      <TableCell>
        <Link
          component={RouterLink}
          // TODO: handle the case where the first failed build does not exist
          // in LUCI Bisection; currently, this link will navigate to a page
          // that will display an error alert
          to={`/ui/bisection/analysis/b/${analysis.firstFailedBbid}`}
          data-testid="analysis_table_row_analysis_link"
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
        {builderLink && (
          <Link
            href={builderLink.url}
            target="_blank"
            rel="noreferrer"
            underline="always"
            data-testid="analysis_table_row_builder_link"
          >
            {builderLink.linkText}
          </Link>
        )}
      </TableCell>
      <CulpritsTableCell
        culprits={analysis.culprits}
        status={analysis.status}
      />
    </TableRow>
  );
}
