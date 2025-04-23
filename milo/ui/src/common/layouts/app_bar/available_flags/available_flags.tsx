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
import { useState } from 'react';

import { StyledIconButton } from '@/common/components/gm3_styled_components';
import {
  FeatureFlag,
  useAvailableFlags,
  useGetFlagStatus,
} from '@/common/feature_flags/';

export function AvailableFlags() {
  const availableFlags = useAvailableFlags();
  const getFlagStatus = useGetFlagStatus();
  const [open, setOpen] = useState(false);
  const flagValues = [...availableFlags.values()];

  const handleClickOpen = () => {
    setOpen(true);
  };

  const handleClose = () => {
    setOpen(false);
  };

  function handleFlagStatusChange(flag: FeatureFlag, value: boolean) {
    const flagStatus = getFlagStatus(flag);
    if (flagStatus) {
      flagStatus.observers.forEach((observer) => {
        observer(value ? 'on' : 'off');
      });
    }
  }

  return (
    <>
      <StyledIconButton
        onClick={handleClickOpen}
        color="inherit"
        role="button"
        aria-label="Toggle feature flags"
        title={
          availableFlags.size === 0
            ? 'No available flags'
            : 'Toggle feature flags'
        }
        disabled={availableFlags.size === 0}
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
              {flagValues.map((flag) => (
                <TableRow
                  key={`${flag.status.flag.config.namespace}:${flag.status.flag.config.name}`}
                >
                  <TableCell>
                    {`${flag.status.flag.config.namespace}:${flag.status.flag.config.name}`}
                  </TableCell>
                  <TableCell>{flag.status.flag.config.description}</TableCell>
                  <TableCell>
                    <Switch
                      title={`${flag.status.flag.config.namespace}:${flag.status.flag.config.name} switch`}
                      onChange={(e) =>
                        handleFlagStatusChange(
                          flag.status.flag,
                          e.target.checked,
                        )
                      }
                      checked={flag.status.activeStatus}
                    />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </>
  );
}
