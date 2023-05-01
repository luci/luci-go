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
import Typography from '@mui/material/Typography';

import { PlainTable } from '../plain_table/plain_table';

import { Analysis } from '../../services/luci_bisection';
import {
  EMPTY_LINK,
  ExternalLink,
  linkToBuild,
  linkToBuilder,
  linkToCommit,
  linkToCommitRange,
} from '../../tools/link_constructors';
import { AnalysisStatusInfo } from '../status_info/status_info';
import { getFormattedTimestamp } from '../../tools/timestamp_formatters';

interface Props {
  analysis: Analysis;
}

function getSuspectRange(analysis: Analysis): ExternalLink[] {
  if (analysis.culprits && analysis.culprits.length > 0) {
    let culpritLinks: ExternalLink[] = [];
    analysis.culprits.forEach((culprit) => {
      culpritLinks.push(linkToCommit(culprit.commit));
    });
    return culpritLinks;
  }

  if (analysis.nthSectionResult) {
    const result = analysis.nthSectionResult;

    if (result.suspect) {
      return [linkToCommit(result.suspect.gitilesCommit)];
    }

    if (
      result.remainingNthSectionRange &&
      result.remainingNthSectionRange.lastPassed &&
      result.remainingNthSectionRange.firstFailed
    ) {
      return [
        linkToCommitRange(
          result.remainingNthSectionRange.lastPassed,
          result.remainingNthSectionRange.firstFailed
        ),
      ];
    }
  }

  return [];
}

function getBugLinks(analysis: Analysis): ExternalLink[] {
  let bugLinks: ExternalLink[] = [];

  // Get the bug links from the actions for each culprit
  if (analysis.culprits) {
    analysis.culprits.forEach((culprit) => {
      if (culprit.culpritAction) {
        culprit.culpritAction.forEach((action) => {
          if (action.actionType === 'BUG_COMMENTED' && action.bugUrl) {
            // TODO: construct short link text for bug
            bugLinks.push({
              linkText: action.bugUrl,
              url: action.bugUrl,
            });
          }
        });
      }
    });
  }

  return bugLinks;
}

export const AnalysisOverview = ({ analysis }: Props) => {
  let buildLink = EMPTY_LINK;
  if (analysis.firstFailedBbid) {
    buildLink = linkToBuild(analysis.firstFailedBbid);
  }

  let builderLink = EMPTY_LINK;
  if (analysis.builder) {
    builderLink = linkToBuilder(analysis.builder);
  }

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
              {analysis.firstFailedBbid && (
                <Link
                  href={buildLink.url}
                  target='_blank'
                  rel='noreferrer'
                  underline='always'
                >
                  {buildLink.linkText}
                </Link>
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Created time</TableCell>
            <TableCell>{getFormattedTimestamp(analysis.createdTime)}</TableCell>
            <TableCell variant='head'>Builder</TableCell>
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
          </TableRow>
          <TableRow>
            <TableCell variant='head'>End time</TableCell>
            <TableCell>{getFormattedTimestamp(analysis.endTime)}</TableCell>
            <TableCell variant='head'>Failure type</TableCell>
            <TableCell>{analysis.buildFailureType}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Status</TableCell>
            <TableCell>
              <AnalysisStatusInfo status={analysis.status}></AnalysisStatusInfo>
              {/* TODO (aredulla): Currently, analyses are only canceled if
                  a later build is successful. If analyses are canceled for
                  other reasons, we will need to store the cancelation reason
                  in the backend and update the UI here to display it.*/}
              {analysis.runStatus === 'CANCELED' &&
              <Typography color='var(--greyed-out-text-color)'>
                (canceled because the builder started passing again)
              </Typography>}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant='head'>Suspect range</TableCell>
            <TableCell>
              {suspectRange.map((suspectLink) => (
                <span className='span-link' key={suspectLink.url}>
                  <Link
                    data-testid='analysis_overview_suspect_range'
                    href={suspectLink.url}
                    target='_blank'
                    rel='noreferrer'
                    underline='always'
                  >
                    {suspectLink.linkText}
                  </Link>
                </span>
              ))}
            </TableCell>
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
                    <span className='span-link' key={bugLink.url}>
                      <Link
                        data-testid='analysis_overview_bug_link'
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
