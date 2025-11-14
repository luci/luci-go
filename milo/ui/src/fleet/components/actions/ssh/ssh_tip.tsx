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

import TerminalIcon from '@mui/icons-material/Terminal';
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material';
import { useState } from 'react';

import { useBot } from '@/fleet/hooks/swarming_hooks';
import { getDimensionValue } from '@/fleet/utils/bots';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

import CodeSnippet from '../../code_snippet/code_snippet';

const ZONE_SATLAB = 'ZONE_SATLAB';

interface SshTipProps {
  hostname: string;
  dutId: string;
}

/**
 * This component gives the user tips on how to SSH into a ChromeOS device.
 */
// TODO: b/408052902 - Based on usage of this component, we can determine if,
// long-term, it makes sense to add more integrated support for SSHing into
// devices.
export function SshTip({ hostname, dutId }: SshTipProps) {
  const [open, setOpen] = useState<boolean>(false);

  const client = useBotsClient(DEVICE_TASKS_SWARMING_HOST);

  const { info: botInfo } = useBot(client, dutId);

  const isUfsZoneSatlab =
    getDimensionValue(botInfo, 'ufs_zone') === ZONE_SATLAB;
  const isHostnameSatlab = hostname.includes('satlab');
  const isSatlab = isUfsZoneSatlab || isHostnameSatlab;

  const droneServer = getDimensionValue(botInfo, 'drone_server');

  const command =
    isSatlab && droneServer
      ? `ssh -o ProxyJump=moblab@${droneServer} root@${hostname}`
      : `ssh ${hostname}`;

  const showWarning = isSatlab && !droneServer;

  return (
    <>
      <Button
        color="primary"
        size="small"
        startIcon={<TerminalIcon />}
        onClick={() => setOpen(true)}
      >
        SSH
      </Button>
      <Dialog onClose={() => setOpen(false)} open={open}>
        <DialogTitle>SSH into {hostname}</DialogTitle>
        <DialogContent>
          {showWarning && (
            <Alert severity="warning">
              This device is a Satlab device, but we were unable to determine
              the drone server. The following SSH instructions may be incorrect.
            </Alert>
          )}
          {!isSatlab && (
            <Alert severity="info">
              When you first SSH into a ChromeOS device, you will need to follow{' '}
              <a
                href="http://go/chromeos-lab-duts-ssh#setup-private-key-and-ssh-config"
                target="_blank"
                rel="noreferrer"
              >
                these setup instructions
              </a>
              .
            </Alert>
          )}
          <p>
            To learn how to SSH into a ChromeOS device, see:{' '}
            <a
              href="http://go/chromeos-lab-duts-ssh"
              target="_blank"
              rel="noreferrer"
            >
              go/chromeos-lab-duts-ssh
            </a>
          </p>

          <p>To SSH:</p>
          <CodeSnippet
            displayText={'$ ' + command}
            copyText={command}
            copyKind="ssh"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)} variant="contained">
            Okay
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
