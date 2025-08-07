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
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
} from '@mui/material';

import { FLEET_BUILDS_SWARMING_HOST } from '@/fleet/utils/builds';
import { ScheduleAutorepairResult } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import CodeSnippet from '../../code_snippet/code_snippet';

export interface SessionInfo {
  sessionId?: string;
  results?: ScheduleAutorepairResult[];
  dutNames?: string[];
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
  sessionInfo: { dutNames = [], results, sessionId },
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
          {plurifyDevices(results?.length || 0)}:
        </p>
        <ul>
          {results?.map((result) => {
            return (
              <li key={result.unitName}>
                <a
                  href={`/ui/fleet/labs/p/chromeos/devices/${result.unitName}`}
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
                    Failed to schedule autorepair: {result.errorMessage}
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
      {results ? finalScreen : confirmationScreen}
    </Dialog>
  );
}
