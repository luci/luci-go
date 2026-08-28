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

import CloseIcon from '@mui/icons-material/Close';
import ScienceIcon from '@mui/icons-material/Science';
import {
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';
import { useMemo, useState } from 'react';

import { StyledIconButton } from '@/common/components/gm3_styled_components';
import {
  FeatureFlag,
  getCurrentEnvironment,
  getFeatureFlagKey,
  getFeatureFlagLocalStorageKey,
  getFeatureFlagValue,
  isFlagAvailableInEnvironment,
  REGISTERED_FLAGS,
  useAvailableFlags,
  useGetFlagStatus,
} from '@/common/feature_flags/';
import { logging } from '@/common/tools/logging';

export function AvailableFlags() {
  const availableFlags = useAvailableFlags();
  const getFlagStatus = useGetFlagStatus();
  const [open, setOpen] = useState(false);
  const [toggleCount, setToggleCount] = useState(0);

  const currentEnv = getCurrentEnvironment();

  const flagDisplayList = useMemo(() => {
    // Combine mounted availableFlags with all REGISTERED_FLAGS
    const allFlagsMap = new Map<string, FeatureFlag>();

    for (const flag of REGISTERED_FLAGS.values()) {
      if (isFlagAvailableInEnvironment(flag, currentEnv)) {
        const key = getFeatureFlagKey(flag);
        allFlagsMap.set(key, flag);
      }
    }

    for (const activeFlag of availableFlags.values()) {
      const flag = activeFlag.status.flag;
      if (isFlagAvailableInEnvironment(flag, currentEnv)) {
        const key = getFeatureFlagKey(flag);
        allFlagsMap.set(key, flag);
      }
    }

    // Reference toggleCount to re-evaluate display list when user toggles flag overrides
    void toggleCount;

    return Array.from(allFlagsMap.values()).map((flag) => {
      const activeFlag = availableFlags.get(flag);
      const activeStatus = activeFlag
        ? activeFlag.status.activeStatus
        : getFeatureFlagValue(flag);

      return {
        flag,
        activeStatus,
      };
    });
  }, [availableFlags, currentEnv, toggleCount]);

  if (flagDisplayList.length === 0) {
    return null;
  }

  const handleClickOpen = () => {
    setOpen(true);
  };

  const handleClose = () => {
    setOpen(false);
  };

  function handleFlagStatusChange(flag: FeatureFlag, value: boolean) {
    const key = getFeatureFlagLocalStorageKey(flag);
    if (typeof window !== 'undefined') {
      try {
        window.localStorage.setItem(key, value ? 'on' : 'off');
      } catch (e) {
        logging.warn(
          'Failed to write feature flag override to localStorage:',
          e,
        );
      }
    }

    const flagStatus = getFlagStatus(flag);
    if (flagStatus) {
      flagStatus.observers.forEach((observer) => {
        observer(value ? 'on' : 'off');
      });
    }

    setToggleCount((c) => c + 1);
  }

  return (
    <>
      <StyledIconButton
        onClick={handleClickOpen}
        color="inherit"
        role="button"
        aria-label="Toggle feature flags"
        title={
          flagDisplayList.length === 0
            ? 'No available flags'
            : 'Toggle feature flags'
        }
        disabled={flagDisplayList.length === 0}
      >
        <ScienceIcon />
      </StyledIconButton>
      <Dialog
        onClose={handleClose}
        aria-labelledby="feature flags dialog"
        open={open}
        fullWidth
        maxWidth="lg"
        scroll="body"
      >
        <DialogTitle sx={{ m: 0, p: 2 }} id="Feature flags">
          Feature flags
        </DialogTitle>
        <IconButton
          aria-label="close"
          onClick={handleClose}
          sx={(theme) => ({
            position: 'absolute',
            right: 8,
            top: 8,
            color: theme.palette.grey[500],
          })}
        >
          <CloseIcon />
        </IconButton>
        <DialogContent dividers>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell width="30%" align="left">
                  Flag namespace-name
                </TableCell>
                <TableCell align="left">Description</TableCell>
                <TableCell align="left" width="10%">
                  Toggle
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {flagDisplayList.map(({ flag, activeStatus }) => {
                const flagKey = getFeatureFlagKey(flag);
                return (
                  <TableRow key={flagKey}>
                    <TableCell>{flagKey}</TableCell>
                    <TableCell>{flag.config.description}</TableCell>
                    <TableCell>
                      <Switch
                        title={`${flagKey} switch`}
                        onChange={(e) =>
                          handleFlagStatusChange(flag, e.target.checked)
                        }
                        checked={activeStatus}
                      />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </>
  );
}
