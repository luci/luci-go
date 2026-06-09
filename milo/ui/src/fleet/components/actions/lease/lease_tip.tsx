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

import VpnKeyIcon from '@mui/icons-material/VpnKey';
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material';
import { useState } from 'react';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import CodeSnippet from '../../code_snippet/code_snippet';

interface LeaseTipProps {
  hostname: string;
}

/**
 * This component gives the user tips on how to lease a ChromeOS device.
 */
export function LeaseTip({ hostname }: LeaseTipProps) {
  const [open, setOpen] = useState<boolean>(false);
  const { trackEvent } = useGoogleAnalytics();

  const command = `crosfleet dut lease -host ${hostname}`;

  const handleClickOpen = () => {
    trackEvent('lease_tip', {
      componentName: 'lease_tip_button',
    });
    setOpen(true);
  };

  return (
    <>
      <Button
        color="primary"
        size="small"
        startIcon={<VpnKeyIcon />}
        onClick={handleClickOpen}
      >
        Lease
      </Button>
      <Dialog onClose={() => setOpen(false)} open={open}>
        <DialogTitle>Lease {hostname}</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            When you first lease a ChromeOS device, you will need to follow{' '}
            <a
              href="http://go/crosfleet-cli#installation-and-updates"
              target="_blank"
              rel="noreferrer"
            >
              these setup instructions
            </a>{' '}
            to install and configure <code>crosfleet</code>.
          </Alert>
          <p>
            To learn how to lease a ChromeOS device, see:{' '}
            <a href="http://go/crosfleet-cli" target="_blank" rel="noreferrer">
              crosfleet CLI Documentation
            </a>
          </p>

          <p>To lease:</p>
          <CodeSnippet
            displayText={command}
            copyText={command}
            copyKind="lease"
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
