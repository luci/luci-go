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

import { Backdrop, Box, Drawer } from '@mui/material';
import { styled } from '@mui/material/styles';
import { useState, useRef, useEffect, useCallback } from 'react';

import { TestAggregationViewer } from '@/test_investigation/components/test_aggregation_viewer/test_aggregation_viewer';
import { useInvocation, useTestVariant } from '@/test_investigation/context';

const StyledDrawer = styled(Drawer)(({ theme }) => ({
  '& .MuiDrawer-paper': {
    top: '64px',
    height: 'calc(100% - 64px)',
    boxSizing: 'border-box',
    borderRight: 'none',
    boxShadow: theme.shadows[4],
  },
}));

const DragHandle = styled('div')(({ theme }) => ({
  position: 'absolute',
  top: 0,
  right: 0,
  bottom: 0,
  width: '5px',
  cursor: 'col-resize',
  backgroundColor: 'transparent',
  zIndex: 1,
  '&:hover': {
    backgroundColor: theme.palette.divider,
  },
}));

export interface TestAggregationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
}

const MIN_DRAWER_WIDTH = 500;

export function TestAggregationDrawer({
  isOpen,
  onClose = () => {},
}: TestAggregationDrawerProps) {
  const invocation = useInvocation();
  const testVariant = useTestVariant();
  const [drawerWidth, setDrawerWidth] = useState(500);
  const isDragging = useRef(false);
  const handleClose = () => {
    if (isOpen) {
      onClose();
    }
  };

  const handleMouseMove = useCallback((e: MouseEvent) => {
    if (!isDragging.current) return;

    // Calculate new width to the right edge
    const newWidth = e.clientX;
    const maxWidth = window.innerWidth * 0.5; // 50vw

    setDrawerWidth(Math.max(MIN_DRAWER_WIDTH, Math.min(newWidth, maxWidth)));
  }, []);

  const handleMouseUp = useCallback(() => {
    isDragging.current = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
  }, [handleMouseMove]);

  const handleMouseDown = (e: React.MouseEvent) => {
    e.preventDefault();
    isDragging.current = true;
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  // Clean up global event listeners on unmount
  useEffect(() => {
    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [handleMouseMove, handleMouseUp]);

  return (
    <>
      <Backdrop
        open={isOpen}
        onClick={handleClose}
        sx={{
          zIndex: (theme) => theme.zIndex.drawer - 1,
          color: '#fff',
          top: '64px',
        }}
      />
      <StyledDrawer
        variant="persistent"
        anchor="left"
        open={isOpen}
        PaperProps={{ style: { width: drawerWidth } }}
      >
        <Box
          sx={{
            overflow: 'hidden',
            height: '100%',
            position: 'relative',
          }}
        >
          <TestAggregationViewer
            invocation={invocation}
            testVariant={testVariant ?? undefined}
            autoLocate={isOpen}
          />
          <DragHandle onMouseDown={handleMouseDown} />
        </Box>
      </StyledDrawer>
    </>
  );
}
