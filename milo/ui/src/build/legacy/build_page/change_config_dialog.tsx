// Copyright 2022 The LUCI Authors.
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
  DialogTitle,
  MenuItem,
  OutlinedInput,
  Select,
  Typography,
} from '@mui/material';
import { untracked } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useEffect, useState } from 'react';

import { useStore } from '@/common/store';

import { BuildPageTab, INITIAL_DEFAULT_TAB, parseTab } from './common';

export interface ChangeConfigDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

// An array of [buildTabName, buildTabLabel] tuples.
// Use an array of tuples instead of an Object to ensure order.
const TAB_NAME_LABEL_TUPLES = Object.freeze([
  [BuildPageTab.Overview, 'Overview'],
  [BuildPageTab.TestResults, 'Test Results'],
  [BuildPageTab.Infra, 'Infra'],
  [BuildPageTab.RelatedBuilds, 'Related Builds'],
  [BuildPageTab.Timeline, 'Timeline'],
  [BuildPageTab.Blamelist, 'Blamelist'],
] as const);

export const ChangeConfigDialog = observer(
  ({ open, onClose, container }: ChangeConfigDialogProps) => {
    const buildConfig = useStore().userConfig.build;
    const [tab, setTabName] = useState(() =>
      untracked(() => parseTab(buildConfig.defaultTab) || INITIAL_DEFAULT_TAB),
    );

    // Sync the local state with the global config whenever the dialog is
    // (re-)opened. Without this
    // 1. the uncommitted config left in the last edit won't be discarded, and
    // 2. config changes due to other reason won't be reflected in the dialog,
    //    causing unintentional overwriting some configs.
    useEffect(() => {
      // No point updating the tab name when the dialog is not shown.
      if (!open) {
        return;
      }
      setTabName(parseTab(buildConfig.defaultTab) || INITIAL_DEFAULT_TAB);
    }, [open, buildConfig]);

    return (
      <Dialog
        onClose={onClose}
        open={open}
        fullWidth
        maxWidth="sm"
        container={container}
      >
        <DialogTitle>Settings</DialogTitle>
        <DialogContent
          sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: 1 }}
        >
          <Typography
            sx={{
              display: 'flex',
              justifyContent: 'center',
              alignContent: 'center',
              flexDirection: 'column',
            }}
          >
            Default tab:
          </Typography>
          <Select
            value={tab}
            onChange={(e) => setTabName(e.target.value as BuildPageTab)}
            input={<OutlinedInput size="small" />}
            MenuProps={{ disablePortal: true }}
            sx={{ width: '180px' }}
          >
            {TAB_NAME_LABEL_TUPLES.map(([tab, label]) => (
              <MenuItem key={tab} value={tab}>
                {label}
              </MenuItem>
            ))}
          </Select>
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} variant="text">
            Dismiss
          </Button>
          <Button
            onClick={() => {
              buildConfig.setDefaultTab(tab);
              onClose?.();
            }}
            variant="contained"
          >
            Confirm
          </Button>
        </DialogActions>
      </Dialog>
    );
  },
);
