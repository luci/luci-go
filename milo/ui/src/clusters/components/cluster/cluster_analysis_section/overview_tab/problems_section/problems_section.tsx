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

import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useContext, useState } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { ClusterContext } from '@/clusters/components/cluster/cluster_context';
import PanelHeading from '@/clusters/components/headings/panel_heading/panel_heading';
import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import { useFetchProjectConfig } from '@/clusters/hooks/use_fetch_project_config';
import useFetchRule from '@/clusters/hooks/use_fetch_rule';
import {
  Problem,
  identifyProblems,
  sortProblemsByDescendingActiveAndPriority,
} from '@/clusters/tools/problems';
import { BugManagement } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';
import { BugManagementState } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

import { ProblemExplanationDialog } from './problem_explanation_dialog/problem_explanation_dialog';
import { ProblemRow } from './problem_row/problem_row';

const problemsTooltipText =
  'The problems this cluster has been identified as causing,' +
  ' based on policies configured by your project. Resolve all active problems to close this bug.';

export const ProblemsSection = () => {
  const clusterId = useContext(ClusterContext);
  if (clusterId.algorithm !== 'rules') {
    throw new Error('problems section should only be shown on rules');
  }

  const {
    isLoading: isConfigLoading,
    data: projectConfig,
    error: configError,
  } = useFetchProjectConfig(clusterId.project);

  const {
    isLoading: isRuleLoading,
    data: rule,
    error: ruleError,
  } = useFetchRule(clusterId.project, clusterId.id);

  return (
    <Box>
      <PanelHeading>
        Problems
        <HelpTooltip text={problemsTooltipText} />
      </PanelHeading>
      {configError && (
        <LoadErrorAlert entityName="project config" error={configError} />
      )}
      {!configError && ruleError && (
        <LoadErrorAlert entityName="rule" error={ruleError} />
      )}
      {!(configError || ruleError) && (isConfigLoading || isRuleLoading) && (
        <CentralizedProgress />
      )}
      {projectConfig && rule && (
        <ProblemsSummary
          bugManagementState={rule.bugManagementState!}
          config={projectConfig.bugManagement}
        ></ProblemsSummary>
      )}
    </Box>
  );
};

interface Props {
  bugManagementState: BugManagementState;
  config: BugManagement | undefined;
}

const ProblemsSummary = ({ bugManagementState, config }: Props) => {
  const [openProblem, setOpenProblem] = useState<Problem | undefined>(
    undefined,
  );

  // Handler for closing the problem explanation dialog.
  const handleClose = () => {
    setOpenProblem(undefined);
  };

  const problems = identifyProblems(config, bugManagementState);
  sortProblemsByDescendingActiveAndPriority(problems);

  return (
    <Box data-testid="problem-summary">
      <ProblemExplanationDialog
        openProblem={openProblem}
        handleClose={handleClose}
      />
      {problems.length > 0 && (
        <Table size="small" sx={{ width: 'initial' }}>
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontSize: '1rem', paddingLeft: '0px' }}>
                Name
              </TableCell>
              <TableCell sx={{ fontSize: '1rem' }}>Status</TableCell>
              <TableCell sx={{ fontSize: '1rem' }}>Priority</TableCell>
              <TableCell sx={{ fontSize: '1rem' }}>Active since</TableCell>
              <TableCell></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {problems.map((p) => {
              return (
                <ProblemRow
                  key={p.policy.id}
                  problem={p}
                  openProblemDialog={setOpenProblem}
                />
              );
            })}
          </TableBody>
        </Table>
      )}
      {config && config.policies && config.policies.length > 0 && (
        <>
          {problems.length === 0 && (
            <Typography>No problems have been identified.</Typography>
          )}
          <Typography sx={{ marginTop: '1rem' }} color="GrayText">
            Problems are identified by{' '}
            <Link
              href="http://goto.google.com/luci-analysis-setup#project-configuration"
              target="_blank"
              rel="noreferrer"
              underline="always"
            >
              policies configured by your project
            </Link>
            .
          </Typography>
        </>
      )}
      {!(config && config.policies && config.policies.length > 0) && (
        <Typography>
          <Link
            href="http://goto.google.com/luci-analysis-setup#project-configuration"
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            Configure bug management policies
          </Link>{' '}
          to surface problems here.
        </Typography>
      )}
    </Box>
  );
};
