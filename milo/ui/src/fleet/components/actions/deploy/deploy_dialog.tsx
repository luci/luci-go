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

import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  Tooltip,
} from '@mui/material';

import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { FLEET_BUILDS_SWARMING_HOST } from '@/fleet/utils/builds';
import { isPartnerNamespace } from '@/fleet/utils/devices';
import { ScheduleDeployResult } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import CodeSnippet from '../../code_snippet/code_snippet';

export interface SessionInfo {
  sessionId?: string;
  results?: ScheduleDeployResult[];
  dutNames?: string[];
  namespaces?: (string | readonly string[])[];
}

export interface DeployDialogProps {
  open: boolean;
  sessionInfo: SessionInfo;
  handleClose: () => void;
  handleOk: () => void;
  loading: boolean;
}

const plurifyDevices = (count: number) => {
  return count === 1 ? 'device' : `${count} devices`;
};

function getDeviceDetailListItem(dutName: string) {
  return (
    <li key={dutName}>
      <a
        href={generateChromeOsDeviceDetailsURL(dutName)}
        target="_blank"
        rel="noreferrer"
      >
        {dutName}
      </a>
    </li>
  );
}

export default function DeployDialog({
  open,
  sessionInfo: { dutNames = [], results, sessionId, namespaces = [] },
  handleClose,
  handleOk,
  loading,
}: DeployDialogProps) {
  const isPartner = namespaces.some(isPartnerNamespace);
  const shivasCommand = `shivas update dut -force-deploy ${dutNames.join(' ')}`;

  const loadingScreen = (
    <>
      <DialogTitle>Deploying devices</DialogTitle>
      <DialogContent>
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            minHeight: '100px',
          }}
        >
          <CircularProgress />
        </Box>
      </DialogContent>
    </>
  );

  const confirmationScreen = isPartner ? (
    <>
      <DialogTitle>Deploying devices</DialogTitle>
      <DialogContent>
        <Stack direction="row" alignItems="center" spacing={1}>
          <Tooltip title="View External Device Manual">
            <IconButton
              size="small"
              href="http://go/satlab-manual"
              target="_blank"
            >
              <HelpOutlineIcon fontSize="small" />
            </IconButton>
          </Tooltip>
          <p>
            At this time, the Fleet Console does not support force deploy for
            external devices.
          </p>
        </Stack>
        <p>
          If this feature is critical to your use case, please comment on or
          upvote{' '}
          <a href="http://b/493273483" target="_blank" rel="noreferrer">
            this feature request
          </a>
          .
        </p>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button onClick={handleOk} variant="contained" disabled>
          Confirm
        </Button>
      </DialogActions>
    </>
  ) : (
    <>
      <DialogTitle>Deploying devices</DialogTitle>
      <DialogContent>
        {dutNames.length > 0 && (
          <>
            <p>
              Please confirm that you want to deploy the following{' '}
              {plurifyDevices(dutNames.length)}:
            </p>
            <ul>
              {dutNames?.map((dutName) => getDeviceDetailListItem(dutName))}
            </ul>
            <p>Equivalent shivas command:</p>
            <CodeSnippet
              displayText={shivasCommand}
              copyText={shivasCommand}
              copyKind="shivas_deploy"
            />
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        {dutNames.length > 0 && (
          <Button onClick={handleOk} variant="contained">
            Confirm
          </Button>
        )}
      </DialogActions>
    </>
  );

  const finalScreen = (
    <>
      <DialogTitle>Deploy results</DialogTitle>
      <DialogContent>
        <p>
          Deploy has been triggered on the following{' '}
          {plurifyDevices(results?.length || 0)}:
        </p>
        <ul>
          {results?.map((result) => {
            return (
              <li key={result.unitName}>
                <a
                  href={generateChromeOsDeviceDetailsURL(result.unitName)}
                  target="_blank"
                  rel="noreferrer"
                >
                  {result.unitName}
                </a>
                {': '}
                {result.taskUrl ? (
                  <a href={result.taskUrl} target="_blank" rel="noreferrer">
                    View in Milo
                  </a>
                ) : (
                  <span style={{ color: 'red' }}>
                    Failed to schedule deploy: {result.errorMessage}
                  </span>
                )}
              </li>
            );
          })}
        </ul>
        <p>
          (
          <a
            href={`https://${FLEET_BUILDS_SWARMING_HOST}/tasklist?f=admin-session:${sessionId}`}
            target="_blank"
            rel="noreferrer"
          >
            View tasks in Swarming
          </a>
          )
        </p>
        <p>
          <i>
            It may take a few minutes for Swarming to update the task to show up
            on Milo.
          </i>
        </p>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} variant="contained">
          Done
        </Button>
      </DialogActions>
    </>
  );

  const getDialogContent = () => {
    if (loading) {
      return loadingScreen;
    }
    if (results) {
      return finalScreen;
    }
    return confirmationScreen;
  };

  return (
    <Dialog onClose={handleClose} open={open} fullWidth maxWidth="sm">
      {getDialogContent()}
    </Dialog>
  );
}
