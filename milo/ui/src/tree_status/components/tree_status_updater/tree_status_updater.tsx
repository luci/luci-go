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

import {
  Alert,
  Button,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  TextField,
} from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import {
  statusColor,
  StatusIcon,
  statusText,
} from '@/common/tools/tree_status/tree_status_utils';
import {
  CreateStatusRequest,
  GeneralState,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';
import { useTreeStatusClient } from '@/tree_status/hooks/prpc_clients';

interface TreeStatusUpdaterProps {
  tree: string;
}

function inferStateFromMessage(message: string): GeneralState {
  const lowerMsg = message.toLowerCase();
  const closed = lowerMsg.includes('close');
  if (closed && lowerMsg.includes('maint')) {
    return GeneralState.MAINTENANCE;
  } else if (lowerMsg.includes('throt')) {
    return GeneralState.THROTTLED;
  } else if (closed) {
    return GeneralState.CLOSED;
  } else {
    return GeneralState.OPEN;
  }
}

// TreeStatusUpdater presents a simple form that allows creating a new status update for a tree.
export const TreeStatusUpdater = ({ tree }: TreeStatusUpdaterProps) => {
  const [state, setState] = useState(GeneralState.OPEN);
  const [message, setMessage] = useState('');
  const [manuallyUpdated, setManuallyUpdated] = useState(false);
  const queryClient = useQueryClient();
  const client = useTreeStatusClient();
  const updateMutation = useMutation({
    mutationFn: (request: CreateStatusRequest) => {
      return client.CreateStatus(request);
    },
    onSuccess: () => {
      setState(GeneralState.OPEN);
      setMessage('');
      setManuallyUpdated(false);
      queryClient.invalidateQueries();
    },
  });
  if (!tree) {
    return null;
  }

  return (
    <>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          margin: '8px 16px 16px',
        }}
      >
        <FormControl style={{ minWidth: '160px' }}>
          <InputLabel>New Status</InputLabel>
          <Select
            label="New Status"
            value={state}
            onChange={(e) => {
              setState(e.target.value as GeneralState);
              setManuallyUpdated(true);
            }}
            disabled={updateMutation.isPending}
            size="small"
            sx={{ color: statusColor(state) }}
            renderValue={(selected) => (
              <div
                style={{ display: 'flex', alignItems: 'center', gap: '8px' }}
              >
                <StatusIcon state={selected} /> {statusText(selected)}
              </div>
            )}
          >
            {[
              GeneralState.OPEN,
              GeneralState.CLOSED,
              GeneralState.THROTTLED,
              GeneralState.MAINTENANCE,
            ].map((status) => (
              <MenuItem
                key={status}
                value={status}
                sx={{
                  color: statusColor(status),
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                }}
              >
                <StatusIcon state={status} /> {statusText(status)}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl style={{ flexGrow: 1 }}>
          <TextField
            variant="outlined"
            label="Message"
            value={message}
            onChange={(e) => {
              setMessage(e.target.value);
              // Try to infer the desired state from the message text, but don't
              // override the user's choice if they manually updated the state
              // via the dropdown.
              if (!manuallyUpdated) {
                setState(inferStateFromMessage(e.target.value));
              }
            }}
            disabled={updateMutation.isPending}
            size="small"
          />
        </FormControl>
        <FormControl>
          <Button
            variant="contained"
            color="primary"
            onClick={() =>
              updateMutation.mutateAsync(
                CreateStatusRequest.fromPartial({
                  parent: `trees/${tree}/status`,
                  status: {
                    generalState: state,
                    message,
                    name: '',
                    createTime: undefined,
                    createUser: '',
                  },
                }),
              )
            }
          >
            Update
          </Button>
        </FormControl>
      </div>
      {updateMutation.isError ? (
        <Alert severity="error">
          <strong>Error updating status:</strong>
          {(updateMutation.error as Error).name}:{' '}
          {(updateMutation.error as Error).message}
        </Alert>
      ) : null}
    </>
  );
};
