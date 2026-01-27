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

import CloseIcon from '@mui/icons-material/Close';
import { Alert, IconButton, Snackbar, styled } from '@mui/material';
import { useEffect, useState } from 'react';
import { useLocalStorage } from 'react-use';

const Key = styled('span')({
  fontFamily: 'monospace',
  backgroundColor: 'rgba(0, 0, 0, 0.1)',
  padding: '1px 5px',
  borderRadius: '4px',
  fontWeight: 'bold',
  marginLeft: '4px',
  marginRight: '4px',
});

export const ShortcutsTip = () => {
  const [dismissed, setDismissed] = useLocalStorage<boolean>(
    'fleet_console_shortcuts_tip_dismissed',
    false,
  );
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (dismissed) {
      return () => {};
    }

    // Small delay to appear after page load
    const timer = setTimeout(() => setOpen(true), 1500);
    return () => clearTimeout(timer);
  }, [dismissed]);

  const handleDismiss = () => {
    setOpen(false);
    setDismissed(true);
  };

  if (dismissed) return null;

  return (
    <Snackbar
      open={open}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
    >
      <Alert
        severity="info"
        sx={{ width: '100%', alignItems: 'center' }}
        action={
          <>
            <IconButton
              size="small"
              aria-label="close"
              color="inherit"
              onClick={handleDismiss}
            >
              <CloseIcon fontSize="small" />
            </IconButton>
          </>
        }
      >
        New! Try pressing <Key>?</Key> for shortcuts
      </Alert>
    </Snackbar>
  );
};
