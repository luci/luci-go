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

import { useGetAccessToken } from '@/common/components/auth_state_provider';
import { PrpcClient } from '@/generic_libs/tools/prpc_client';
import {
  CreateStatusRequest,
  GeneralState,
  TreeStatusClientImpl,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';

interface TreeStatusUpdaterProps {
  tree: string;
}

// TreeStatusUpdater presents a simple form that allows creating a new status update for a tree.
export const TreeStatusUpdater = ({ tree }: TreeStatusUpdaterProps) => {
  const [state, setState] = useState(GeneralState.OPEN);
  const [message, setMessage] = useState('');
  const queryClient = useQueryClient();
  const getAuthToken = useGetAccessToken();
  const updateMutation = useMutation({
    mutationFn: (request: CreateStatusRequest) => {
      const client = new TreeStatusClientImpl(
        new PrpcClient({
          host: SETTINGS.luciTreeStatus.host,
          insecure: false,
          getAuthToken,
        }),
      );
      // eslint-disable-next-line new-cap
      return client.CreateStatus(request);
    },
    onSuccess: () => {
      setState(GeneralState.OPEN);
      setMessage('');
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
        <FormControl style={{ minWidth: '120px' }}>
          <InputLabel>New Status</InputLabel>
          <Select
            label="New Status"
            value={state}
            onChange={(e) => setState(e.target.value as GeneralState)}
            disabled={updateMutation.isLoading}
          >
            <MenuItem value={GeneralState.OPEN}>Open</MenuItem>
            <MenuItem value={GeneralState.CLOSED}>Closed</MenuItem>
            <MenuItem value={GeneralState.THROTTLED}>Throttled</MenuItem>
            <MenuItem value={GeneralState.MAINTENANCE}>Maintenance</MenuItem>
          </Select>
        </FormControl>
        <FormControl style={{ flexGrow: 1 }}>
          <TextField
            variant="outlined"
            label="Message"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            disabled={updateMutation.isLoading}
          />
        </FormControl>
        <FormControl>
          <Button
            variant="contained"
            color="primary"
            onClick={() =>
              updateMutation.mutateAsync({
                parent: `trees/${tree}/status`,
                status: {
                  generalState: state,
                  message,
                  name: '',
                  createTime: undefined,
                  createUser: '',
                },
              })
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
