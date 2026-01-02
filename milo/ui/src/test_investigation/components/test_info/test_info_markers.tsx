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

import { Link, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { Fragment, useMemo } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  SummaryLineItem,
  SummaryLineDivider,
} from '@/common/components/page_summary_line';
import { displayApproxDuration } from '@/common/tools/time_utils/time_utils';
import { OutputTestVerdict } from '@/common/types/verdict';
import { TestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { BuildDescriptor } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import {
  AnyInvocation,
  isRootInvocation,
} from '@/test_investigation/utils/invocation_utils';
import {
  constructBlamelistCommitLink,
  formatAllCLs,
  getBuildDetailsUrl,
  getCommitGitilesUrlFromInvocation,
  getCommitInfoFromInvocation,
  getSourcesFromInvocation,
} from '@/test_investigation/utils/test_info_utils';

import { BuildInfoTooltip } from './build_info_tooltip';

export interface TestInfoMarkersProps {
  invocation: AnyInvocation;
  project?: string;
  testVariant: OutputTestVerdict;
  testVariantBranch?: TestVariantBranch | null;
}

export function TestInfoMarkers({
  invocation,
  project,
  testVariant,
  testVariantBranch,
}: TestInfoMarkersProps) {
  const elements = useMemo(() => {
    const infoItems: React.ReactElement[] = [];
    const sources = getSourcesFromInvocation(invocation);

    // 1. Changelists
    const cls = formatAllCLs(sources?.changelists);
    if (cls.length > 0) {
      const label = cls.length === 1 ? 'CL' : 'Changelists';
      infoItems.push(
        <SummaryLineItem key="cls" label={label}>
          {cls.map((cl, i) => (
            <Fragment key={cl.key}>
              {i > 0 && ', '}
              <Link href={cl.url} target="_blank" rel="noopener noreferrer">
                {cl.display}
              </Link>
            </Fragment>
          ))}
        </SummaryLineItem>,
      );
    }

    // 2. Commit
    const commitInfo = getCommitInfoFromInvocation(invocation);
    if (commitInfo) {
      const originalCommitLink = getCommitGitilesUrlFromInvocation(invocation);
      const blamelistCommitLink = constructBlamelistCommitLink(
        project,
        testVariant,
        testVariantBranch,
        invocation,
      );
      infoItems.push(
        <SummaryLineItem
          key="commit"
          label={cls.length > 0 ? 'Commit' : 'Baseline'}
        >
          <Link
            href={blamelistCommitLink || originalCommitLink}
            target="_blank"
            rel="noopener noreferrer"
          >
            {commitInfo}
          </Link>
        </SummaryLineItem>,
      );
    }

    // 3. Build setup
    let primaryBuild;
    let extraBuilds: readonly BuildDescriptor[];
    if (isRootInvocation(invocation)) {
      primaryBuild = invocation?.primaryBuild?.androidBuild;
      extraBuilds = invocation?.extraBuilds;
    } else {
      primaryBuild = invocation?.properties?.primaryBuild;
      extraBuilds = invocation.properties?.extraBuilds;
    }

    // 4. Primary Build
    const primaryBuildId = primaryBuild?.buildId;
    const primaryTarget = primaryBuild?.buildTarget;
    const primaryBranch = primaryBuild?.branch;
    if (primaryBuildId && primaryTarget) {
      infoItems.push(
        <SummaryLineItem key="primary-build" label="Build">
          <HtmlTooltip
            title={
              <BuildInfoTooltip
                buildId={primaryBuildId}
                buildTarget={primaryTarget}
                buildBranch={primaryBranch || ''}
              />
            }
          >
            <Link
              href={getBuildDetailsUrl(primaryBuildId, primaryTarget)}
              target="_blank"
              rel="noopener noreferrer"
            >
              {primaryBuildId}
            </Link>
          </HtmlTooltip>
        </SummaryLineItem>,
      );
    }

    // 5. Extra Builds
    if (extraBuilds && extraBuilds.length > 0) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const eb0: any = extraBuilds[0];
      const build0 = isRootInvocation(invocation) ? eb0.androidBuild : eb0;
      const buildId0 = build0?.buildId;
      const target0 = build0?.buildTarget;
      const branch0 = build0?.branch;

      if (buildId0 && target0) {
        if (extraBuilds.length === 1) {
          infoItems.push(
            <SummaryLineItem key="extra-build" label="Extra build">
              <HtmlTooltip
                title={
                  <BuildInfoTooltip
                    buildId={buildId0}
                    buildTarget={target0}
                    buildBranch={branch0 || ''}
                  />
                }
              >
                <Link
                  href={getBuildDetailsUrl(buildId0, target0)}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {buildId0}
                </Link>
              </HtmlTooltip>
            </SummaryLineItem>,
          );
        } else {
          infoItems.push(
            <Typography
              key="extra-builds"
              variant="body2"
              component="span"
              color="text.secondary"
            >
              <HtmlTooltip
                title={
                  <BuildInfoTooltip
                    buildId={buildId0}
                    buildTarget={target0}
                    buildBranch={branch0 || ''}
                  />
                }
              >
                <Link
                  href={getBuildDetailsUrl(buildId0, target0)}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {`${extraBuilds.length} extra builds`}
                </Link>
              </HtmlTooltip>
            </Typography>,
          );
        }
      }
    }

    // 6. Fallback Time
    if (infoItems.length === 0 && invocation.createTime) {
      const createTime = DateTime.fromISO(invocation.createTime);
      const diff = DateTime.now().diff(createTime);
      infoItems.push(
        <SummaryLineItem key="fallback-time" label="Created">
          {displayApproxDuration(diff)} ago
        </SummaryLineItem>,
      );
    }

    const interleaved: React.ReactElement[] = [];
    infoItems.forEach((res, i) => {
      if (i > 0) {
        interleaved.push(<SummaryLineDivider key={`div-${i}`} />);
      }
      interleaved.push(res);
    });
    return interleaved;
  }, [invocation, project, testVariant, testVariantBranch]);

  if (elements.length === 0) {
    return null;
  }

  return <>{elements}</>;
}
