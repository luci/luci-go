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

import { makeClusterLink } from '@/analysis/tools/utils';
import { DurationBadge } from '@/common/components/duration_badge';
import { getUniqueBugs } from '@/common/tools/cluster_utils/cluster_utils';
import { parseProtoDuration } from '@/common/tools/time_utils';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { ArtifactLink } from '@/test_verdict/components/artifact_link';
import { parseInvId } from '@/test_verdict/tools/invocation_utils';
import { parseTestResultName } from '@/test_verdict/tools/utils';

import { useProject } from '../../../context';
import { useClustersByResultId } from '../../context';
import { useCombinedArtifacts, useResult } from '../context';

import { ResultSummary } from './result_summary';

interface LogArtifactsProps {
  testhausLogs?: Artifact;
  stainlessLogs?: Artifact;
}

function LogsArtifacts({ stainlessLogs, testhausLogs }: LogArtifactsProps) {
  const bothExist = testhausLogs && stainlessLogs;
  return (
    <>
      View logs in:&nbsp;
      {testhausLogs && (
        <>
          <ArtifactLink artifact={testhausLogs} />
        </>
      )}
      {bothExist && <>, </>}
      {stainlessLogs && <ArtifactLink artifact={stainlessLogs} />}
    </>
  );
}

export function ResultBasicInfo() {
  const [expanded, setExpanded] = useState(true);
  const result = useResult();
  const clustersByResultId = useClustersByResultId(result.resultId);
  const project = useProject();
  const artifacts = useCombinedArtifacts();

  const testhausLogs = artifacts.find((a) => a.artifactId === 'testhaus_logs');
  const stainlessLogs = artifacts.find(
    (a) => a.artifactId === 'stainless_logs',
  );

  const parsedResultName = parseTestResultName(result.name);
  const parsedInvId = parseInvId(parsedResultName.invocationId);
  // There can be at most one failureReason cluster.
  const reasonCluster = clustersByResultId?.filter((c) =>
    c.clusterId.algorithm.startsWith('reason-'),
  )?.[0];

  const uniqueBugs =
    clustersByResultId &&
    getUniqueBugs(
      clustersByResultId
        .map((c) => c.bug)
        .filter((b) => !!b) as Array<AssociatedBug>,
    );

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
        <Grid container rowGap={1}>
          <Grid item container columnGap={1} alignItems="center">
            {result.duration && (
              <DurationBadge duration={parseProtoDuration(result.duration)} />
            )}
            {result.duration && parsedInvId.type === 'swarming-task' && (
              <Divider orientation="vertical" />
            )}
            {parsedInvId.type === 'swarming-task' && (
              <>
                Swarming Task:
                <Link
                  href={getSwarmingTaskURL(
                    parsedInvId.swarmingHost,
                    parsedInvId.taskId,
                  )}
                  target="_blank"
                  rel="noreferrer"
                >
                  {parsedInvId.taskId}
                </Link>
              </>
            )}
          </Grid>
          {result.failureReason && (
            <Grid item container columnGap={1} alignItems="center">
              <Grid item>
                Failure reason&nbsp;
                {reasonCluster && project && (
                  <Link
                    target="_blank"
                    href={makeClusterLink(project, reasonCluster.clusterId)}
                  >
                    (similar failures)
                  </Link>
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
          {result.summaryHtml && (
            <Grid item container rowGap={1} direction="column">
              <Grid item>Summary:</Grid>
              <ResultSummary
                summaryHtml={result.summaryHtml}
                resultName={result.name}
                invId={parsedResultName.invocationId}
              />
            </Grid>
          )}
          <Grid container item>
            {(testhausLogs || stainlessLogs) && (
              <LogsArtifacts
                testhausLogs={testhausLogs}
                stainlessLogs={stainlessLogs}
              />
            )}
          </Grid>
          {uniqueBugs && uniqueBugs.length > 0 && (
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
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
}
