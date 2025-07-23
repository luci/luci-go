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
import {
  Alert,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
} from '@mui/material';

import {
  generateBuildUrl,
  BuildIdentifier,
  FLEET_BUILDS_SWARMING_HOST,
} from '@/fleet/utils/builds';

import CodeSnippet from '../../code_snippet/code_snippet';

export interface SessionInfo {
  sessionId?: string;
  builds?: BuildIdentifier[];
  dutNames?: string[];
  invalidDutNames?: string[];
}

export interface AutorepairDialogProps {
  open: boolean;
  sessionInfo: SessionInfo;
  handleClose: () => void;
  handleOk: () => void;
  deepRepair: boolean;
  handleDeepRepairChange: (checked: boolean) => void;
}

const plurifyDevices = (count: number) => {
  return count === 1 ? 'device' : `${count} devices`;
};

function getDeviceDetailListItem(dutName: string) {
  return (
    <li key={dutName}>
      <a
        href={`/ui/fleet/labs/p/chromeos/devices/${dutName}`}
        target="_blank"
        rel="noreferrer"
      >
        {dutName}
      </a>
    </li>
  );
}

export default function AutorepairDialog({
  open,
  sessionInfo: { dutNames = [], builds, sessionId, invalidDutNames = [] },
  handleClose,
  handleOk,
  deepRepair,
  handleDeepRepairChange,
}: AutorepairDialogProps) {
  const shivasCommand = `shivas repair${deepRepair ? ' -deep' : ''} ${dutNames.join(' ')}`;
  const confirmationScreen = (
    <>
      <DialogTitle>Running autorepair</DialogTitle>
      <DialogContent>
        {/* TODO: b/394429368 - remove this alert. */}
        <Alert severity="info">
          At this time, devices in the <code>ready</code> and{' '}
          <code>needs_repair</code> states cannot have autorepair run on them
          from the UI. For more info, see:{' '}
          <a href="http://b/394429368" target="_blank" rel="noreferrer">
            b/394429368
          </a>
        </Alert>

        {invalidDutNames.length > 0 && (
          <>
            <Alert severity="error" sx={{ mt: 1 }}>
              For the following devices autorepair will not be executed, as they
              are in a <code>ready</code> and/or <code>needs_repair</code>{' '}
              state:
              <ul>
                {invalidDutNames?.map((dutName) =>
                  getDeviceDetailListItem(dutName),
                )}
              </ul>
            </Alert>
          </>
        )}

        {dutNames.length > 0 && (
          <>
            <p>
              Please confirm that you want to run autorepair on the following{' '}
              {plurifyDevices(dutNames.length)}:
            </p>
            <ul>
              {dutNames?.map((dutName) => getDeviceDetailListItem(dutName))}
            </ul>
            <FormControlLabel
              control={
                <Checkbox
                  checked={deepRepair}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                    handleDeepRepairChange(e.target.checked)
                  }
                />
              }
              style={{ marginBottom: '8px', marginLeft: '24px' }}
              label="Deep repair these devices"
            />

            <p>Equivalent shivas command:</p>
            <CodeSnippet
              displayText={'$ ' + shivasCommand}
              copyText={shivasCommand}
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
      <DialogTitle>Autorepair results</DialogTitle>
      <DialogContent>
        <p>
          Autorepair has been triggered on the following{' '}
          {plurifyDevices(dutNames.length)}:
        </p>
        <ul>
          {builds?.map((b, i) => {
            const dutName = dutNames[i];
            return (
              <li key={dutName}>
                <a
                  href={`/ui/fleet/labs/p/chromeos/devices/${dutName}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {dutName}
                </a>
                :{' '}
                <a href={generateBuildUrl(b)} target="_blank" rel="noreferrer">
                  View in Milo
                </a>
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
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} variant="contained">
          Done
        </Button>
      </DialogActions>
    </>
  );
  return (
    <Dialog onClose={handleClose} open={open}>
      {builds ? finalScreen : confirmationScreen}
    </Dialog>
  );
}
