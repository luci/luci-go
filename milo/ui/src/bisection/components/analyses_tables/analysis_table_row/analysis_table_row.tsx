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

import CommentIcon from '@mui/icons-material/Comment';
import ReplayIcon from '@mui/icons-material/Replay';
import { Typography } from '@mui/material';
import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { Link as RouterLink } from 'react-router-dom';

import { getCommitShortHash } from '@/bisection/tools/commit_formatters';
import { EMPTY_LINK, linkToBuilder } from '@/bisection/tools/link_constructors';
import {
  getFormattedDuration,
  getFormattedTimestamp,
} from '@/bisection/tools/timestamp_formatters';
import {
  Analysis,
  AnalysisStatus,
  Culprit,
  CulpritActionType,
} from '@/common/services/luci_bisection';

interface CulpritActionIconProps {
  actionType: CulpritActionType;
}

const CulpritActionIcon = ({ actionType }: CulpritActionIconProps) => {
  switch (actionType) {
    case 'CULPRIT_AUTO_REVERTED':
      return (
        <ReplayIcon
          fontSize="small"
          sx={{ paddingRight: '0.25rem', color: 'var(--failure-color)' }}
          data-testid="culprit-action-icon-auto-reverted"
        />
      );
    case 'REVERT_CL_CREATED':
      return (
        <ReplayIcon
          fontSize="small"
          sx={{ paddingRight: '0.25rem', color: 'var(--warning-text-color)' }}
          data-testid="culprit-action-icon-revert-created"
        />
      );
    case 'CULPRIT_CL_COMMENTED':
      return (
        <CommentIcon
          fontSize="small"
          sx={{ paddingRight: '0.25rem', color: 'var(--light-text-color)' }}
          data-testid="culprit-action-icon-culprit-commented"
        />
      );
    case 'EXISTING_REVERT_CL_COMMENTED':
      return (
        <CommentIcon
          fontSize="small"
          sx={{ paddingRight: '0.25rem', color: 'var(--warning-text-color)' }}
          data-testid="culprit-action-icon-revert-commented"
        />
      );
    default:
      return <></>;
  }
};

interface CulpritSpanProps {
  culprit: Culprit;
}

const CulpritSpan = ({ culprit }: CulpritSpanProps) => {
  let description = getCommitShortHash(culprit.commit.id);
  if (culprit.reviewTitle) {
    description += `: ${culprit.reviewTitle}`;
  }

  return (
    <span className="span-link">
      <Link
        data-testid="analysis_table_row_culprit_link"
        href={culprit.reviewUrl}
        target="_blank"
        rel="noreferrer"
        underline="always"
      >
        <Typography display="flex" variant="inherit">
          {culprit.culpritAction?.map((action) => (
            <CulpritActionIcon
              actionType={action.actionType}
              key={action.actionType}
            ></CulpritActionIcon>
          ))}
          {description}
        </Typography>
      </Link>
    </span>
  );
};

interface CulpritsTableCellProps {
  culprits: Culprit[] | undefined;
  status: AnalysisStatus;
}

const CulpritsTableCell = ({ culprits, status }: CulpritsTableCellProps) => {
  if (culprits == null || culprits.length == 0) {
    return (
      <TableCell>
        <span className="data-placeholder">
          {status === 'SUSPECTFOUND' && 'Suspect found (not verified)'}
        </span>
      </TableCell>
    );
  }

  return (
    <TableCell>
      {culprits.map((culprit) => (
        <CulpritSpan culprit={culprit} key={culprit.reviewUrl}></CulpritSpan>
      ))}
    </TableCell>
  );
};

interface AnalysisTableProps {
  analysis: Analysis;
}

export const AnalysisTableRow = ({ analysis }: AnalysisTableProps) => {
  let builderLink = EMPTY_LINK;
  if (analysis.builder) {
    builderLink = linkToBuilder(analysis.builder);
  }

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
        {analysis.builder && (
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
};
