// Copyright 2024 The LUCI Authors.
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

import HelpOutline from '@mui/icons-material/HelpOutline';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { HeuristicAnalysis } from '@/monitoring/util/server_json';

interface HeuristicResultProps {
  heuristicResult: HeuristicAnalysis;
}

export const HeuristicResult = ({ heuristicResult }: HeuristicResultProps) => {
  const suspects = heuristicResult?.suspects ?? [];
  if (suspects.length === 0) {
    return (
      <>
        LUCI Bisection couldn&apos;t find any heuristic suspect for this
        failure.
      </>
    );
  }
  const heuristicTooltipTitle =
    'The heuristic suspects were based on best-guess effort, so they may not be 100% accurate.';
  return (
    <TableContainer>
      <Table sx={{ maxWidth: '1000px', tableLayout: 'fixed' }}>
        <TableHead>
          <TableRow>
            <TableCell align="left" sx={{ width: '350px' }}>
              Heuristic result
              {/* We dont use Tooltip from MUI here as the MUI tooltip is attached in <body> tag
                  so style cannot be applied. */}
              <span title={heuristicTooltipTitle}>
                <IconButton>
                  <HelpOutline></HelpOutline>
                </IconButton>
              </span>
            </TableCell>
            <TableCell align="left" sx={{ width: '60px' }}>
              Confidence
            </TableCell>
            <TableCell align="left">Justification</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {suspects.map((s) => (
            <TableRow key={s.reviewUrl}>
              <TableCell align="left">
                <Link
                  href={s.reviewUrl}
                  target="_blank"
                  rel="noopener"
                  onClick={() => {
                    gtag('event', 'ClickSuspectLink', {
                      event_category: 'LuciBisection',
                      event_label: s.reviewUrl,
                      transport_type: 'beacon',
                    });
                  }}
                >
                  {s.reviewUrl}
                </Link>
              </TableCell>
              <TableCell align="left">
                {confidenceText(s.confidence_level)}
              </TableCell>
              <TableCell align="left">
                <pre style={{ whiteSpace: 'pre-wrap' }}>
                  {shortenJustification(s.justification)}
                </pre>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

// Sometimes justification is too long if a CL touches many files.
// In such case we should shorten the justification to at most 3 lines
// and link to the detailed analysis if the sheriff wants to see the details
// (when the detail analysis page is ready).
function shortenJustification(justification: string) {
  const lines = justification.split('\n');
  if (lines.length < 4) {
    return justification;
  }
  return lines.slice(0, 3).join('\n') + '\n...';
}

function confidenceText(confidenceLevel: number) {
  switch (confidenceLevel) {
    case 1:
      return 'Low';
    case 2:
      return 'Medium';
    case 3:
      return 'High';
    default:
      return 'N/A';
  }
}
