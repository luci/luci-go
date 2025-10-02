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

import MenuIcon from '@mui/icons-material/Menu';
import { Drawer, IconButton, Paper, Tooltip, useTheme } from '@mui/material';
import { useState } from 'react';

import { TestDrawerProvider } from './context'; // Import from the context directory
import { DrawerContent } from './drawer_content';

const DRAWER_WIDTH_OPEN = 600;

export function TestNavigationDrawer() {
  const theme = useTheme();
  const [isOpen, setIsOpen] = useState(false);

  const handleToggleDrawer = () => setIsOpen(!isOpen);

  return (
    <>
      <Paper
        elevation={isOpen ? 0 : 4}
        sx={{
          position: 'fixed',
          top: '20%',
          left: isOpen ? DRAWER_WIDTH_OPEN : 0,
          transform: 'translateY(-50%)',
          zIndex: theme.zIndex.drawer + (isOpen ? 1 : 2),
          borderTopLeftRadius: 0,
          borderBottomLeftRadius: 0,
          borderTopRightRadius: '8px',
          borderBottomRightRadius: '8px',
          transition: theme.transitions.create('left', {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.enteringScreen,
          }),
          bgcolor: 'background.paper',
        }}
      >
        <Tooltip
          title={isOpen ? 'Close navigation' : 'Open navigation'}
          placement="right"
        >
          <IconButton
            sx={{
              color: 'var(--blue-600, #1A73E8)',
            }}
            onClick={handleToggleDrawer}
            size="small"
          >
            <MenuIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Paper>

      <Drawer
        variant="temporary"
        anchor="left"
        open={isOpen}
        onClose={handleToggleDrawer}
        sx={{
          width: DRAWER_WIDTH_OPEN,
          flexShrink: 0,
          '& .MuiDrawer-paper': {
            width: DRAWER_WIDTH_OPEN,
            boxSizing: 'border-box',
            height: '100%',
            boxShadow: theme.shadows[5],
            overflow: 'hidden',
          },
        }}
        ModalProps={{ keepMounted: true }}
      >
        <TestDrawerProvider>
          <DrawerContent />
        </TestDrawerProvider>
      </Drawer>
    </>
  );
}
