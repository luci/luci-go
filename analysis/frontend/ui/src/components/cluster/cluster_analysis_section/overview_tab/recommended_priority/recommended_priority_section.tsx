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

import {
  useContext,
  useState,
} from 'react';

import CloseIcon from '@mui/icons-material/Close';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Typography from '@mui/material/Typography';

import CentralizedProgress from '@/components/centralized_progress/centralized_progress';
import { ClusterContext } from '@/components/cluster/cluster_context';
import PanelHeading from '@/components/headings/panel_heading/panel_heading';
import HelpTooltip from '@/components/help_tooltip/help_tooltip';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import useFetchCluster from '@/hooks/use_fetch_cluster';
import { useFetchProjectConfig } from '@/hooks/use_fetch_project_config';
import { ClusterMetrics } from '@/services/cluster';
import { Metric } from '@/services/metrics';
import { ProjectConfig } from '@/services/project';

import { OverviewTabContextData } from '../overview_tab_context';
import {
  constructCriteriumLabel,
  PriorityExplanationSection,
} from './priority_explanation_section/priority_explanation_section';
import {
  createPriorityRecommendation,
  PriorityThreshold,
} from './priority_recommendation';

const priorityTooltipText = "The priority with which LUCI Analysis recommends actioning this cluster, based on the cluster's impact and project configuration.";

export const RecommendedPrioritySection = () => {
  const clusterId = useContext(ClusterContext);
  const { metrics } = useContext(OverviewTabContextData);

  const {
    isLoading: isConfigLoading,
    data: projectConfig,
    error: configError,
  } = useFetchProjectConfig(clusterId.project);

  const {
    isLoading: isClusterLoading,
    error: clusterError,
    data: cluster,
  } = useFetchCluster(clusterId.project, clusterId.algorithm, clusterId.id);

  return (
    <Box>
      <PanelHeading>
        Recommended Priority<HelpTooltip text={priorityTooltipText} />
      </PanelHeading>
      {configError && (
        <LoadErrorAlert entityName="project config" error={configError} />
      )}
      {!configError && clusterError && (
        <LoadErrorAlert entityName="cluster" error={clusterError} />
      )}
      {!(configError || clusterError) &&
        (isConfigLoading || isClusterLoading) && (
          <CentralizedProgress />
        )
      }
      {projectConfig && cluster && metrics && (
        <RecommendedPrioritySummary
          metricValues={cluster?.metrics || {}}
          metrics={metrics}
          projectConfig={projectConfig} ></RecommendedPrioritySummary>
      )}
    </Box>
  );
};

interface Props {
  metricValues: ClusterMetrics;
  metrics: Metric[];
  projectConfig: ProjectConfig;
}

const RecommendedPrioritySummary = ({ metricValues, metrics, projectConfig }: Props) => {
  const [open, setOpen] = useState(false);

  // Handlers for opening/closing the bug priority recommendation dialog.
  const handleOpenClicked = () => {
    setOpen(true);
  };
  const handleClose = () => {
    setOpen(false);
  };

  const priorities: PriorityThreshold[] = ensurePriorityPrefix(
    getPriorityThresholds(projectConfig),
  );
  const recommendation = createPriorityRecommendation(
    metricValues,
    metrics,
    priorities,
  );

  const recommendedPriority = recommendation.recommendation;
  const priorityLabel = recommendedPriority?.priority || "N/A";
  const satisfiedCriteria = recommendedPriority?.criteria.filter(c => c.satisfied) || [];

  const isHighPriority = (priorityLabel === "P0" || priorityLabel === "P1");

  return (
    <Box data-testid="recommended-priority-summary">
      {
        recommendedPriority ?
          <Chip
            color={isHighPriority ? "primary" : "default"
            }
            variant={priorityLabel === "P0" ? "filled" : "outlined"}
            label={
              <Typography
                sx={{ fontWeight: isHighPriority ? "bold" : "regular" }}
                color="inherit"
                variant="h6" >
                {priorityLabel}
              </Typography>
            } /> :
          <Typography
            color="var(--greyed-out-text-color)" >
            {priorityLabel}
          </Typography>
      }
      <ul>
        {satisfiedCriteria.length > 0 && satisfiedCriteria.map((criterium) =>
          <li key={criterium.metricName}>
            <Typography>
              {constructCriteriumLabel(criterium)}
            </Typography>
          </li>
        )}
      </ul>
      <Chip
        aria-label="Recommended priority justification"
        variant="outlined"
        color="default"
        onClick={handleOpenClicked}
        label={
          <Typography variant="button">more info</Typography>
        }
        sx={{ borderRadius: 1 }} />
      <Dialog open={open} onClose={handleClose} maxWidth="lg" fullWidth>
        <DialogTitle>
          Why is the recommended bug priority {priorityLabel}?
          <IconButton
            aria-label="Close recommended priority justification"
            onClick={handleClose}
            sx={{
              position: "absolute",
              right: 8,
              top: 8,
              color: (theme) => theme.palette.grey[500],
            }}>
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent>
          <PriorityExplanationSection
            recommendation={recommendation} />
        </DialogContent>
        <DialogActions>
          <Button
            data-testid="priority-explanation-dialog-close"
            onClick={handleClose}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

function ensurePriorityPrefix(priorities: PriorityThreshold[], prefix: string = "P"): PriorityThreshold[] {
  const upperPrefix = prefix.toUpperCase();

  let prefixedPriorities: PriorityThreshold[] = [];
  priorities.forEach(priorityThreshold => {
    let priorityName = priorityThreshold.priority.trim().toUpperCase();
    if (!priorityName.startsWith(upperPrefix)) {
      priorityName = upperPrefix + priorityName;
    }
    prefixedPriorities.push({
      priority: priorityName,
      thresholds: priorityThreshold.thresholds,
    });
  });

  return prefixedPriorities;
}

function getPriorityThresholds(projectConfig: ProjectConfig): PriorityThreshold[] {
  const buganizerPriorities = projectConfig.buganizer?.priorityMappings;
  const monorailPriorities = projectConfig.monorail?.priorities;

  // Return the priority criteria for the specified bug system,
  // if it is available in the project config.
  if (projectConfig.bugSystem === "BUGANIZER" && buganizerPriorities) {
    return buganizerPriorities;
  } else if (projectConfig.bugSystem === "MONORAIL" && monorailPriorities) {
    return monorailPriorities;
  }

  // When the bug system has not been specified, prefer Buganizer configs
  // over Monorail.
  return buganizerPriorities || monorailPriorities || [];
}
