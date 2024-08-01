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

import Link from '@mui/material/Link';

import {
  GitilesCommit,
  NthSectionAnalysis,
  RegressionRange,
} from '@/monitoring/util/server_json';

interface NthSectionResultProps {
  nth_section_result: NthSectionAnalysis | undefined;
}

export const NthSectionResult = ({
  nth_section_result,
}: NthSectionResultProps) => {
  if (nth_section_result === undefined) {
    return <></>;
  }
  if (
    !nth_section_result.suspect &&
    !nth_section_result.remaining_nth_section_range
  ) {
    return <>Nthsection could not find any suspects</>;
  }

  if (nth_section_result.suspect) {
    return (
      <>
        Nthsection suspect: &nbsp;
        <Link
          href={nth_section_result.suspect.reviewUrl}
          target="_blank"
          rel="noopener"
          onClick={() => {
            gtag('event', 'ClickSuspectLink', {
              event_category: 'LuciBisection',
              event_label: nth_section_result.suspect!.reviewUrl,
              transport_type: 'beacon',
            });
          }}
        >
          {nth_section_result.suspect.reviewTitle ??
            nth_section_result.suspect.reviewUrl}
        </Link>
      </>
    );
  }

  if (nth_section_result.remaining_nth_section_range) {
    const rr = nth_section_result.remaining_nth_section_range;
    const rrLink = linkForRegressionRange(rr);
    return (
      <>
        Nthsection remaining regression range: &nbsp;
        <Link
          href={rrLink}
          target="_blank"
          rel="noopener"
          onClick={() => {
            gtag('event', 'ClickRegressionLink', {
              event_category: 'LuciBisection',
              event_label: rrLink,
              transport_type: 'beacon',
            });
          }}
        >
          {displayForRegressionRange(rr)}
        </Link>
      </>
    );
  }
  return <></>;
};

function linkForRegressionRange(rr: RegressionRange): string {
  return `https://${rr.last_passed.host}/${rr.last_passed.project}/+log/${rr.last_passed.id}..${rr.first_failed.id}`;
}

function displayForRegressionRange(rr: RegressionRange): string {
  return shortHash(rr.last_passed) + ':' + shortHash(rr.first_failed);
}

function shortHash(commit: GitilesCommit): string {
  return commit.id.substring(0, 7);
}
