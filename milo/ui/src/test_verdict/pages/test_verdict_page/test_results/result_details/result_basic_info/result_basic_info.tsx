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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Divider from '@mui/material/Divider';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';
import { Fragment, useState } from 'react';

import { DurationBadge } from '@/common/components/duration_badge';
import { makeClusterLink } from '@/common/services/luci_analysis';
import { TestResult, parseTestResultName } from '@/common/services/resultdb';
import { getClustersUniqueBugs } from '@/common/tools/cluster_utils/cluster_utils';
import { parseProtoDurationStr } from '@/common/tools/time_utils';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import { parseSwarmingTaskFromInvId } from '@/common/tools/utils';

import { useProject } from '../../../context';
import { useClustersByResultId } from '../../context';

interface Props {
  result: TestResult;
}

export function ResultBasicInfo({ result }: Props) {
  const [expanded, setExpanded] = useState(true);
  const clustersByResultId = useClustersByResultId(result.resultId);
  const project = useProject();

  const parsedResultName = parseTestResultName(result.name);
  const swarmingTaskId = parseSwarmingTaskFromInvId(
    parsedResultName.invocationId,
  );
  // There can be at most one failureReason cluster.
  const reasonCluster = clustersByResultId?.filter((c) =>
    c.clusterId.algorithm.startsWith('reason-'),
  )?.[0];

  const uniqueBugs =
    clustersByResultId && getClustersUniqueBugs(clustersByResultId);

  return (
    <Accordion
      variant="outlined"
      disableGutters
      expanded={expanded}
      onChange={() => setExpanded(!expanded)}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Grid item container columnGap={1} alignItems="center">
          {expanded || !result.failureReason ? (
            <Typography>Details</Typography>
          ) : (
            <Grid
              className="failure-bg"
              item
              sx={{
                p: 1,
              }}
            >
              {result.failureReason.primaryErrorMessage}
            </Grid>
          )}
        </Grid>
      </AccordionSummary>
      <AccordionDetails>
        <Grid container rowGap={2}>
          <Grid item container columnGap={1} alignItems="center">
            {result.duration && (
              <DurationBadge
                duration={parseProtoDurationStr(result.duration)}
              />
            )}
            {swarmingTaskId !== null && (
              <>
                <Divider orientation="vertical" />
                Swarming Task:
                <Link
                  href={getSwarmingTaskURL(
                    swarmingTaskId.swarmingHost,
                    swarmingTaskId.taskId,
                  )}
                  target="_blank"
                  rel="noreferrer"
                >
                  {swarmingTaskId.taskId}
                </Link>
              </>
            )}
          </Grid>
          {result.failureReason && (
            <Grid item container columnGap={1} alignItems="center">
              <Grid item>
                Failure reason
                {reasonCluster && project && (
                  <>
                    (
                    <Link
                      target="_blank"
                      href={makeClusterLink(project, reasonCluster.clusterId)}
                    >
                      similar failures
                    </Link>
                    )
                  </>
                )}
                :
              </Grid>
              <Grid
                className="failure-bg"
                item
                sx={{
                  p: 1,
                }}
              >
                {result.failureReason.primaryErrorMessage}
              </Grid>
            </Grid>
          )}
          {uniqueBugs && (
            <Grid item container columnGap={1}>
              Related bugs:
              {uniqueBugs.map((bug, i) => (
                <Fragment key={bug.id}>
                  {i > 0 && i < uniqueBugs.length - 1 && ', '}
                  <Link href={bug.url} target="_blank">
                    {bug.linkText}
                  </Link>
                </Fragment>
              ))}
            </Grid>
          )}
          {/** TODO(b/308716449): Display last successful patchset of the CL. */}
          <Grid item container columnGap={1}>
            Last successful CV run:
            <Link href="#">PATCHSET 3 - Attempt 2</Link>
          </Grid>
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
}
