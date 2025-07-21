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

import FilterListIcon from '@mui/icons-material/FilterList';
import { ListItemIcon, MenuItem, Typography } from '@mui/material';
import Popover from '@mui/material/Popover';
import { GridColumnMenuItemProps } from '@mui/x-data-grid';
import { useState } from 'react';

export function FilterItem(_props: GridColumnMenuItemProps) {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  const handleOpen = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const open = Boolean(anchorEl);
  return (
    <div>
      <MenuItem onClick={handleOpen}>
        <ListItemIcon>
          <FilterListIcon fontSize="small" />
        </ListItemIcon>
        <Typography variant="inherit">Filter</Typography>
      </MenuItem>
      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
      >
        <p>Filter options will be here.</p>
      </Popover>
    </div>
  );
}
