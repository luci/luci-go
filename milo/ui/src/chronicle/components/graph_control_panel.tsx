// Copyright 2026 The LUCI Authors.
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
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Typography,
} from '@mui/material';
import { Panel as ReactFlowPanel } from '@xyflow/react';
import { useContext } from 'react';
import { useNavigate, useParams } from 'react-router';

import { TURBO_CI_ENVIRONMENTS } from '@/common/hooks/grpc_query/turbo_ci/turbo_ci';

import { WorkflowType } from '../fake_turboci_graph';

import {
  ChronicleContext,
  DEMO_ENVIRONMENT_NAME,
  DEMO_WORKPLAN_ID,
} from './context';
import { UseNodeSearchResult } from './hooks/use_node_search';
import { NodeSearchBox } from './node_search_box';

export interface GraphControlPanelProps {
  search: UseNodeSearchResult;
  showAssignmentEdges: boolean;
  onShowAssignmentEdgesChange: (checked: boolean) => void;
  autoFitSelection: boolean;
  onAutoFitSelectionChange: (checked: boolean) => void;
  showCriticalPath: boolean;
  onShowCriticalPathChange: (checked: boolean) => void;
  onCollapseAllSuccessful: () => void;
  onCollapseAll: () => void;
  onExpandAll: () => void;
}

/**
 * Top-left control panel overlay in GraphView containing node search, environment/workflow
 * selectors, display toggles, and collapsible group actions.
 */
export function GraphControlPanel({
  search,
  showAssignmentEdges,
  onShowAssignmentEdgesChange,
  autoFitSelection,
  onAutoFitSelectionChange,
  showCriticalPath,
  onShowCriticalPathChange,
  onCollapseAllSuccessful,
  onCollapseAll,
  onExpandAll,
}: GraphControlPanelProps) {
  const { workplanId: urlWorkplanId } = useParams<{ workplanId: string }>();
  const navigate = useNavigate();
  const {
    workplanId,
    workflowType,
    setWorkflowType,
    selectedNodeId,
    setSelectedNodeId,
    activeEnvironment,
    setActiveEnvironment,
    foundEnvironments,
  } = useContext(ChronicleContext);

  const {
    searchQuery,
    matchedNodeIds,
    safeMatchIndex,
    isCurrentMatchSelected,
    handleSearchChange,
    handleClearSearch,
    handleNextMatch,
    handlePrevMatch,
    stepToMatch,
    cancelPendingMatchNav,
  } = search;

  return (
    <ReactFlowPanel position="top-left">
      <Paper
        elevation={2}
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 0,
          p: 1,
          borderRadius: 1,
        }}
      >
        <NodeSearchBox
          searchQuery={searchQuery}
          matchedNodeIds={matchedNodeIds}
          safeMatchIndex={safeMatchIndex}
          isCurrentMatchSelected={isCurrentMatchSelected}
          selectedNodeId={selectedNodeId}
          onSearchChange={handleSearchChange}
          onClearSearch={handleClearSearch}
          onNextMatch={handleNextMatch}
          onPrevMatch={handlePrevMatch}
          onStepToMatch={stepToMatch}
        />
        <FormControl fullWidth size="small" sx={{ mt: 2 }}>
          <InputLabel id="environment-select-label">Environment</InputLabel>
          <Select
            labelId="environment-select-label"
            id="environment-select"
            value={activeEnvironment || ''}
            label="Environment"
            onChange={(e) => {
              const environment = e.target.value;
              if (environment === DEMO_ENVIRONMENT_NAME) {
                const currentPath = window.location.pathname;
                const idToReplace = urlWorkplanId || workplanId;
                const newPath = currentPath.replace(
                  new RegExp(`/chronicle/${idToReplace}(?=/|$)`),
                  '/chronicle/' + DEMO_WORKPLAN_ID,
                );
                navigate(newPath);
              } else {
                setActiveEnvironment(environment);
              }
            }}
          >
            {[
              ...foundEnvironments.filter(
                (env) => env.environment !== DEMO_ENVIRONMENT_NAME,
              ),
              TURBO_CI_ENVIRONMENTS.find(
                (env) => env.environment === DEMO_ENVIRONMENT_NAME,
              ),
            ]
              .filter((env): env is NonNullable<typeof env> => Boolean(env))
              .map((env) => (
                <MenuItem key={env.host} value={env.environment}>
                  {env.environment}
                </MenuItem>
              ))}
          </Select>
        </FormControl>
        {workplanId === DEMO_WORKPLAN_ID && (
          <FormControl fullWidth size="small" sx={{ mt: 2 }}>
            <InputLabel id="workflow-type-select-label">
              Workflow Type
            </InputLabel>
            <Select
              labelId="workflow-type-select-label"
              id="workflow-type-select"
              value={workflowType}
              label="Workflow Type"
              onChange={(e) => setWorkflowType(e.target.value as WorkflowType)}
            >
              <MenuItem value={WorkflowType.ANDROID}>Android</MenuItem>
              <MenuItem value={WorkflowType.BROWSER}>Browser</MenuItem>
              <MenuItem value={WorkflowType.BROWSER_FUTURE}>
                Browser Future
              </MenuItem>
              <MenuItem value={WorkflowType.ANDROID_GIGANTIC_POSTSUBMIT}>
                Android Gigantic Postsubmit
              </MenuItem>
            </Select>
          </FormControl>
        )}
        <FormControlLabel
          control={
            <Checkbox
              checked={showAssignmentEdges}
              onChange={(e) => onShowAssignmentEdgesChange(e.target.checked)}
            />
          }
          label={<Typography variant="body2">Show Assignment Edges</Typography>}
        />
        <FormControlLabel
          control={
            <Checkbox
              checked={autoFitSelection}
              onChange={(e) => {
                const checked = e.target.checked;
                cancelPendingMatchNav();
                onAutoFitSelectionChange(checked);
                if (
                  checked &&
                  !selectedNodeId &&
                  isCurrentMatchSelected &&
                  matchedNodeIds.length > 0
                ) {
                  setSelectedNodeId(matchedNodeIds[safeMatchIndex]);
                }
              }}
            />
          }
          label={<Typography variant="body2">Fit View on Selection</Typography>}
        />
        <FormControlLabel
          control={
            <Checkbox
              checked={showCriticalPath}
              onChange={(e) => onShowCriticalPathChange(e.target.checked)}
            />
          }
          label={<Typography variant="body2">Show Critical Path</Typography>}
        />
        <Button onClick={onCollapseAllSuccessful} sx={{ mt: 1 }} size="small">
          Collapse Successful
        </Button>
        <Button onClick={onCollapseAll} size="small">
          Collapse All
        </Button>
        <Button onClick={onExpandAll} size="small">
          Expand All
        </Button>
      </Paper>
    </ReactFlowPanel>
  );
}
