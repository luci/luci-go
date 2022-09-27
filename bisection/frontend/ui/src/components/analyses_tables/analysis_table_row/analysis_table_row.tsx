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

import { Link as RouterLink } from 'react-router-dom';

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import {
  Analysis,
  AnalysisStatus,
  Culprit,
} from '../../../services/luci_bisection';
import { getCommitShortHash } from '../../../tools/commit_formatters';
import { EMPTY_LINK, linkToBuilder } from '../../../tools/link_constructors';
import {
  getFormattedDuration,
  getFormattedTimestamp,
} from '../../../tools/timestamp_formatters';

interface AnalysisTableProps {
  analysis: Analysis;
}

interface CulpritsTableCellProps {
  culprits: Culprit[] | undefined;
  status: AnalysisStatus;
}

const CulpritsTableCell = ({ culprits, status }: CulpritsTableCellProps) => {
  if (culprits == null || culprits.length === 0) {
    return (
      <TableCell className='data-placeholder'>
        {status === 'FOUND' && 'Culprit found (not verified)'}
      </TableCell>
    );
  }

  const culpritLinks = culprits.map((culprit) => {
    let description = getCommitShortHash(culprit.commit.id);
    if (culprit.reviewTitle) {
      description += `: ${culprit.reviewTitle}`;
    }
    return {
      linkText: description,
      url: culprit.reviewUrl,
    };
  });

  return (
    <TableCell>
      {culpritLinks.map((culpritLink) => (
        <span className='span-link' key={culpritLink.url}>
          <Link
            data-testid='analysis_table_row_culprit_link'
            href={culpritLink.url}
            target='_blank'
            rel='noreferrer'
            underline='always'
          >
            {culpritLink.linkText}
          </Link>
        </span>
      ))}
    </TableCell>
  );
};

export const AnalysisTableRow = ({ analysis }: AnalysisTableProps) => {
  let builderLink = EMPTY_LINK;
  if (analysis.builder) {
    builderLink = linkToBuilder(analysis.builder);
  }

  return (
    <TableRow hover data-testid='analysis_table_row'>
      <TableCell>
        <Link
          component={RouterLink}
          // TODO: handle the case where the first failed build does not exist
          // in LUCI Bisection; currently, this link will navigate to a page
          // that will display an error alert
          to={`/analysis/b/${analysis.firstFailedBbid}`}
          data-testid='analysis_table_row_analysis_link'
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
            data-testid='analysis_table_row_builder_link'
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
};
