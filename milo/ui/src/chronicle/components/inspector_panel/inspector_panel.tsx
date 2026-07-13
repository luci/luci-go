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
import {
  Box,
  Divider,
  FormControlLabel,
  IconButton,
  Paper,
  Switch,
  Typography,
} from '@mui/material';
import { ReactElement, useState } from 'react';

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { CheckDetails } from './check_details';
import { StageDetails } from './stage_details';
import { RenderMode } from './types';

export interface InspectorPanelProps {
  nodeId: string;
  nodeLabel?: string;
  viewData?: Check | Stage;
  valueDataMap?: Map<string, ValueData>;
  onClose: () => void;
}

// Type guards to discriminate between Check and Stage
function isCheckView(view: Check | Stage): view is Check {
  return (view as Check).kind !== undefined;
}

function isStageView(view: Check | Stage): view is Stage {
  return (view as Stage).assignments !== undefined;
}

export function InspectorPanel({
  nodeId,
  viewData,
  valueDataMap,
  onClose,
}: InspectorPanelProps) {
  const [renderMode, setRenderMode] = useState<RenderMode>(
    RenderMode.Structured,
  );
  let title = '';
  let detailsComponent: ReactElement | undefined = undefined;

  if (viewData && valueDataMap) {
    if (isCheckView(viewData)) {
      title = 'Check Details';
      detailsComponent = (
        <CheckDetails
          check={viewData}
          valueDataMap={valueDataMap}
          renderMode={renderMode}
        />
      );
    } else if (isStageView(viewData)) {
      title = 'Stage Details';
      detailsComponent = (
        <StageDetails
          view={viewData}
          valueDataMap={valueDataMap}
          renderMode={renderMode}
        />
      );
    }
  }

  return (
    <Paper
      elevation={1}
      sx={{
        width: '100%',
        height: '100%',
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
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {viewData && (
            <FormControlLabel
              control={
                <Switch
                  checked={renderMode === RenderMode.Json}
                  onChange={(e) =>
                    setRenderMode(
                      e.target.checked
                        ? RenderMode.Json
                        : RenderMode.Structured,
                    )
                  }
                  color="primary"
                  size="small"
                />
              }
              label="Raw JSON"
              labelPlacement="start"
              sx={{
                m: 0,
                '& .MuiFormControlLabel-label': { fontSize: '0.85rem' },
              }}
            />
          )}
          <IconButton
            onClick={onClose}
            size="small"
            aria-label="close inspector"
          >
            <CloseIcon />
          </IconButton>
        </Box>
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
