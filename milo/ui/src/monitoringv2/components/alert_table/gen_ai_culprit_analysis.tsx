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

import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import { Link } from '@mui/material';
import { Link as RouterLink } from 'react-router';

import { GenericSuspect } from '@/bisection/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

export interface GenAIProps {
  analysis: Analysis | undefined;
}

export const GenAiCulpritAnalysis = ({ analysis }: GenAIProps) => {
  if (!analysis?.genAiResult) {
    return <></>;
  }
  const suspect: GenericSuspect = GenericSuspect.fromGenAi(
    analysis!.genAiResult!.suspect!,
  );
  const buildbucketid = analysis!.firstFailedBbid || '';
  if (!suspect) {
    return <></>;
  }
  return (
    <div
      css={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}
    >
      <div
        css={{
          display: 'flex',
          justifyContent: 'space-between',
          flexDirection: 'column',
        }}
      >
        <span
          css={{
            display: 'flex',
            justifyContent: 'space-between',
            flexDirection: 'row',
            paddingBottom: '6px',
          }}
        >
          <div>Suspect: </div>
          <HtmlTooltip title={suspect.reviewTitle}>
            <Link
              href={suspect.reviewUrl}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
              style={{ paddingLeft: '6px' }}
            >
              {suspect.commit.id.slice(0, 10)}
            </Link>
          </HtmlTooltip>
        </span>

        {buildbucketid && (
          <Link
            component={RouterLink}
            to={`/ui/bisection/analysis/b/${buildbucketid}`}
          >
            See Analysis
          </Link>
        )}
      </div>

      <HtmlTooltip title={suspect.verificationDetails.status}>
        <CheckCircleOutlineIcon
          style={{ color: 'var(--success-color)' }}
          sx={{
            verticalAlign: 'middle',
            color: 'rgba(0, 0, 0, 0.54)',
          }}
        ></CheckCircleOutlineIcon>
      </HtmlTooltip>
    </div>
  );
};
