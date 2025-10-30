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

import { Link, TableCell } from '@mui/material';
import { Link as RouterLink } from 'react-router';

import { GenericSuspect } from '@/bisection/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';

export function AnalysisHeadCell() {
  return (
    <HtmlTooltip
      title={'Is a culprit or suspected culprit causing the build failure.'}
    >
      <TableCell width="1px">Culprit Analysis</TableCell>
    </HtmlTooltip>
  );
}

export interface AnalysisCellProps {
  readonly suspect?: GenericSuspect;
  readonly bbid?: string;
}

export function AnalysisContentCell({ suspect, bbid }: AnalysisCellProps) {
  if (!suspect?.verificationDetails || !bbid) {
    return <TableCell />;
  }

  let verificationStatus = 'Suspect';
  let verificationStatusText = `Verification status: ${suspect.verificationDetails!.status}`;

  if (suspect.verificationDetails!.status === 'Confirmed Culprit') {
    verificationStatus = 'Culprit';
    verificationStatusText = '';
  }

  return (
    // TODO(445562638): Add Justification as a tooltip
    <HtmlTooltip
      title={
        <div>
          Identified as {verificationStatus.toLowerCase()} by {suspect.type}
          <br />
          <br />
          {verificationStatusText && (
            <div>
              {verificationStatusText} <br /> <br />
            </div>
          )}
          Click for full analysis.
        </div>
      }
    >
      <TableCell data-testid="culprit" sx={{ minWidth: '30px' }}>
        <Link
          component={RouterLink}
          to={`/ui/bisection/compile-analysis/b/${bbid}`}
        >
          {verificationStatus}
        </Link>
      </TableCell>
    </HtmlTooltip>
  );
}
