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

import './analysis_overview.css';

import Link from '@mui/material/Link';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';

import { Analysis } from '../../services/luci_bisection';
import { PlainTable } from '../plain_table/plain_table';

import {
  ExternalLink,
  linkToBuild,
  linkToCommit,
  linkToCommitRange,
} from '../../tools/link_constructors';

interface Props {
  analysis: Analysis;
}

function getSuspectRange(analysis: Analysis): ExternalLink {
  if (analysis.culprit) {
    return linkToCommit(analysis.culprit);
  }

  if (analysis.nthSectionResult) {
    const result = analysis.nthSectionResult;

    if (result.culprit) {
      return linkToCommit(result.culprit);
    }

    if (result.remainingNthSectionRange) {
      return linkToCommitRange(
        result.remainingNthSectionRange.lastPassed,
        result.remainingNthSectionRange.firstFailed
      );
    }
  }

  return {
    linkText: '',
    url: '',
  };
}

function getBugLinks(analysis: Analysis): ExternalLink[] {
  let bugLinks: ExternalLink[] = [];

  if (analysis.culpritAction) {
    analysis.culpritAction.forEach((action) => {
      if (action.actionType === 'BUG_COMMENTED' && action.bugUrl) {
        // TODO: construct short link text for bug
        bugLinks.push({
          linkText: action.bugUrl,
          url: action.bugUrl,
        });
      }
    });
  }

  return bugLinks;
}

export const AnalysisOverview = ({ analysis }: Props) => {
  const buildLink = linkToBuild(analysis.firstFailedBbid);
  const suspectRange = getSuspectRange(analysis);
  const bugLinks = getBugLinks(analysis);
  return (
    <TableContainer>
      <PlainTable>
        <colgroup>
          <col style={{ width: '15%' }} />
          <col style={{ width: '35%' }} />
          <col style={{ width: '15%' }} />
          <col style={{ width: '35%' }} />
        </colgroup>
        <TableBody data-testid='analysis_overview_table_body'>
          <TableRow>
            <TableCell variant='head'>Analysis ID</TableCell>
            <TableCell>{analysis.analysisId}</TableCell>
            <TableCell variant='head'>Buildbucket ID</TableCell>
            <TableCell>
              <Link
                href={buildLink.url}
                target='_blank'
                rel='noreferrer'
                underline='always'
              >
                {buildLink.linkText}
              </Link>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Status</TableCell>
            <TableCell>{analysis.status}</TableCell>
            <TableCell variant='head'>Builder</TableCell>
            <TableCell>
              {`${analysis.builder.project}/${analysis.builder.bucket}/${analysis.builder.builder}`}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Suspect range</TableCell>
            <TableCell>
              <Link
                data-testid='analysis_overview_suspect_range'
                href={suspectRange.url}
                target='_blank'
                rel='noreferrer'
                underline='always'
              >
                {suspectRange.linkText}
              </Link>
            </TableCell>
            <TableCell variant='head'>Failure type</TableCell>
            <TableCell>{analysis.buildFailureType}</TableCell>
          </TableRow>
          {bugLinks.length > 0 && (
            <>
              <TableRow>
                <TableCell>
                  <br />
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell variant='head'>Related bugs</TableCell>
                <TableCell colSpan={3}>
                  {bugLinks.map((bugLink) => (
                    <span className='bugLink' key={bugLink.url}>
                      <Link
                        href={bugLink.url}
                        target='_blank'
                        rel='noreferrer'
                        underline='always'
                      >
                        {bugLink.linkText}
                      </Link>
                    </span>
                  ))}
                </TableCell>
              </TableRow>
            </>
          )}
        </TableBody>
      </PlainTable>
    </TableContainer>
  );
};
