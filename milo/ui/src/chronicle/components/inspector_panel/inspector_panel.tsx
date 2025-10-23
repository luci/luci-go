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

import CloseIcon from '@mui/icons-material/Close';
import { Box, Divider, IconButton, Paper, Typography } from '@mui/material';
import { ReactElement } from 'react';

import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { CheckDetails } from './check_details';
import { StageDetails } from './stage_details';

export interface InspectorPanelProps {
  nodeId: string;
  nodeLabel?: string;
  viewData?: CheckView | StageView;
  onClose: () => void;
}

// Type guards to discriminate between CheckView and StageView
function isCheckView(view: CheckView | StageView): view is CheckView {
  return (view as CheckView).check !== undefined;
}

function isStageView(view: CheckView | StageView): view is StageView {
  return (view as StageView).stage !== undefined;
}

export function InspectorPanel({
  nodeId,
  viewData,
  onClose,
}: InspectorPanelProps) {
  let title = '';
  let detailsComponent: ReactElement | undefined = undefined;

  if (viewData) {
    if (isCheckView(viewData)) {
      title = 'Check Details';
      detailsComponent = <CheckDetails view={viewData} />;
    } else if (isStageView(viewData)) {
      title = 'Stage Details';
      detailsComponent = <StageDetails view={viewData} />;
    }
  }

  return (
    <Paper
      elevation={1}
      sx={{
        width: '400px',
        height: '100%',
        borderLeft: '1px solid #e0e0e0',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Box
        sx={{
          p: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          bgcolor: '#f5f5f5',
        }}
      >
        <Typography variant="h6" component="div" sx={{ fontWeight: 'bold' }}>
          {title}
        </Typography>
        <IconButton onClick={onClose} size="small" aria-label="close inspector">
          <CloseIcon />
        </IconButton>
      </Box>
      <Divider />
      <Box sx={{ p: 2, overflowY: 'auto', flexGrow: 1 }}>
        {detailsComponent || (
          <Typography variant="body2" color="text.secondary">
            No information available for {nodeId}
          </Typography>
        )}
      </Box>
    </Paper>
  );
}
