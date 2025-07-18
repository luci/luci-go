// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import {
  Button,
  List,
  ListItemButton,
  ListItemText,
  Popover,
  Tooltip,
  Typography,
} from '@mui/material';
import _ from 'lodash';
import { useState } from 'react';

import { usePlatform, platformRenderString } from '@/fleet/hooks/usePlatform';
import { colors } from '@/fleet/theme/colors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const PLATFORMS: Platform[] = [Platform.CHROMEOS, Platform.ANDROID];

export function PlatformSelector() {
  const platform = usePlatform();
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handlePlatformSelect = (newPlatform: Platform) => {
    platform.setPlatform(newPlatform);
    handleClose();
  };

  if (!platform.inPlatformScope) {
    return null;
  }

  return (
    <>
      <Tooltip title="Switch platform">
        <Button
          variant="contained"
          onClick={handleClick}
          disableElevation
          endIcon={<ArrowDropDownIcon />}
          sx={{
            color: colors.grey[800],
            backgroundColor: colors.white,
            border: '1px solid',
            borderColor: colors.grey[300],
            paddingBottom: '5px',
            '&:hover': {
              backgroundColor: colors.grey[200],
            },
          }}
        >
          <Typography variant="body1">
            {platformRenderString(platform.platform) || 'Invalid platform'}
          </Typography>
        </Button>
      </Tooltip>
      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
      >
        <List>
          {PLATFORMS.map((p) => (
            <ListItemButton key={p} onClick={() => handlePlatformSelect(p)}>
              <ListItemText primary={platformRenderString(p)} />
            </ListItemButton>
          ))}
        </List>
      </Popover>
    </>
  );
}
