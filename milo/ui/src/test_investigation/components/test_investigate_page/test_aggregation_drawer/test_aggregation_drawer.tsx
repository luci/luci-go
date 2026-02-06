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

import { Backdrop, Box, Drawer } from '@mui/material';
import { styled } from '@mui/material/styles';

import { TestAggregationViewer } from '@/test_investigation/components/test_aggregation_viewer/test_aggregation_viewer';

const StyledDrawer = styled(Drawer)(({ theme }) => ({
  '& .MuiDrawer-paper': {
    width: '500px',
    top: '64px',
    height: 'calc(100% - 64px)',
    boxSizing: 'border-box',
    borderRight: 'none',
    boxShadow: theme.shadows[4],
  },
}));

export interface TestAggregationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
}

export function TestAggregationDrawer({
  isOpen,
  onClose = () => {},
}: TestAggregationDrawerProps) {
  const handleClose = () => {
    if (isOpen) {
      onClose();
    }
  };

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
      <StyledDrawer variant="persistent" anchor="left" open={isOpen}>
        <Box sx={{ overflow: 'auto', height: '100%' }}>
          <TestAggregationViewer />
        </Box>
      </StyledDrawer>
    </>
  );
}
