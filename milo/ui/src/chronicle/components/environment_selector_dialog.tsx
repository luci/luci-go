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
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
} from '@mui/material';

import { TurboCIEnvironment } from '@/common/hooks/grpc_query/turbo_ci/turbo_ci';

import { FailedEnvironment, formatFailedEnvironments } from './context';

export interface EnvironmentSelectorDialogProps {
  open: boolean;
  detectedEnvironments: TurboCIEnvironment[];
  onSelect: (environment: string) => void;
  onClose: () => void;
  requestedEnvFailed?: string;
  failedEnvironments?: FailedEnvironment[];
}

export function EnvironmentSelectorDialog({
  open,
  detectedEnvironments,
  onSelect,
  onClose,
  requestedEnvFailed,
  failedEnvironments,
}: EnvironmentSelectorDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Select Orchestrator Environment</DialogTitle>
      <DialogContent>
        {requestedEnvFailed && (
          <DialogContentText color="error" sx={{ mb: 2, fontWeight: 'bold' }}>
            Workplan was not found in the requested environment:{' '}
            {requestedEnvFailed}.
          </DialogContentText>
        )}
        {failedEnvironments && failedEnvironments.length > 0 && (
          <DialogContentText
            color="warning.main"
            sx={{ mb: 2, fontSize: '0.875rem', fontWeight: 'medium' }}
          >
            Warning: The following environments could not be checked due to
            timeouts/errors: {formatFailedEnvironments(failedEnvironments)}
          </DialogContentText>
        )}
        <DialogContentText>
          This workplan was found in multiple environments. Please select which
          one to load:
        </DialogContentText>
        <List sx={{ pt: 1 }}>
          {detectedEnvironments.map((env) => (
            <ListItem disableGutters key={env.host}>
              <ListItemButton onClick={() => onSelect(env.environment)}>
                <ListItemText primary={env.environment} secondary={env.host} />
              </ListItemButton>
            </ListItem>
          ))}
        </List>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="primary">
          Cancel
        </Button>
      </DialogActions>
    </Dialog>
  );
}
