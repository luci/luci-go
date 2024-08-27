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

import { Close } from '@mui/icons-material';
import { Box, Dialog, DialogContent, DialogTitle } from '@mui/material';
import { ReactNode } from 'react';

import { useLogGroupListDispatch } from '../context';

export interface LogDialogBaseProps {
  readonly dialogHeader: ReactNode;
  readonly children: ReactNode;
}

export function LogDialogBase({ dialogHeader, children }: LogDialogBaseProps) {
  const dispatch = useLogGroupListDispatch();

  return (
    <Dialog
      open
      onClose={() => dispatch({ type: 'dismiss' })}
      maxWidth={false}
      fullWidth
      disableScrollLock
      sx={{
        '& .MuiDialog-paper': {
          margin: 0,
          maxWidth: '100%',
          width: '100%',
          maxHeight: '70%',
          height: '70%',
          position: 'absolute',
          bottom: 0,
          borderRadius: 0,
          overscrollBehavior: 'contain',
        },
      }}
    >
      <DialogTitle
        sx={{
          padding: 0,
          display: 'grid',
          gridTemplateColumns: '1fr 24px',
          borderBottom: 'solid 1px var(--divider-color)',
          backgroundColor: 'var(--block-background-color)',
          fontSize: '16px',
        }}
      >
        <Box>{dialogHeader}</Box>
        <Box
          title="press esc to close the log group overlay"
          onClick={() => dispatch({ type: 'dismiss' })}
        >
          <Close
            css={{
              color: 'red',
              cursor: 'pointer',
              verticalAlign: 'bottom',
              paddingBottom: '4px',
            }}
          />
        </Box>
      </DialogTitle>
      <DialogContent>{children}</DialogContent>
    </Dialog>
  );
}
