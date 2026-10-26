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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import CheckIcon from '@mui/icons-material/Check';
import PlaceIcon from '@mui/icons-material/Place';
import {
  Button,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Popover,
  Tooltip,
} from '@mui/material';
import { useState } from 'react';

import {
  borders,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';

import { REPAIR_LABS, RepairLab, useRepairLab } from './use_repair_lab';

const LAB_LABELS: Record<RepairLab, { name: string; location: string }> = {
  MTV: { name: 'MTV', location: 'Mountain View' },
  HTL: { name: 'HTL', location: 'Hotrod' },
};

export interface LabSelectorProps {
  value?: RepairLab;
  onChange?: (lab: RepairLab) => void;
}

export const LabSelector = ({
  value: propValue,
  onChange: propOnChange,
}: LabSelectorProps) => {
  const { lab: contextLab, setLab: contextSetLab } = useRepairLab();
  const lab = propValue ?? contextLab;
  const setLab = propOnChange ?? contextSetLab;

  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleLabSelect = (selectedLab: RepairLab) => {
    setLab(selectedLab);
    handleClose();
  };

  return (
    <>
      <Tooltip title="Switch lab">
        <Button
          variant="toolbar"
          onClick={handleClick}
          startIcon={<PlaceIcon sx={{ fontSize: iconSizes.control }} />}
          endIcon={<ArrowDropDownIcon />}
          aria-label="Select Lab"
          data-testid="repair-lab-select"
        >
          {`Lab: ${lab}`}
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
        slotProps={{
          paper: {
            elevation: 2,
            sx: { mt: 0.75, border: borders.default, minWidth: 200 },
          },
        }}
      >
        <List sx={{ p: 0.75 }}>
          {REPAIR_LABS.map((item) => {
            const selected = item === lab;
            const info = LAB_LABELS[item];
            return (
              <ListItemButton
                key={item}
                selected={selected}
                onClick={() => handleLabSelect(item)}
                data-testid={`repair-lab-option-${item}`}
                sx={{
                  borderRadius: radii.sm,
                  py: 0.75,
                  px: 1.25,
                  mb: 0.25,
                  '&.Mui-selected': {
                    backgroundColor: fleetColors.primaryContainer,
                    color: fleetColors.primary,
                    '&:hover': {
                      backgroundColor: fleetColors.primaryContainerHover,
                    },
                  },
                }}
              >
                <ListItemIcon sx={{ minWidth: 28 }}>
                  <PlaceIcon
                    sx={{
                      fontSize: iconSizes.control,
                      color: selected
                        ? fleetColors.primary
                        : fleetColors.textSecondary,
                    }}
                  />
                </ListItemIcon>
                <ListItemText
                  primary={`${info.name} (${info.location})`}
                  primaryTypographyProps={{
                    fontFamily: fontFamilies.display,
                    fontSize: fontSizes.body,
                    fontWeight: selected ? 600 : 500,
                    color: selected
                      ? fleetColors.primary
                      : fleetColors.textPrimary,
                  }}
                />
                {selected && (
                  <CheckIcon
                    sx={{
                      fontSize: iconSizes.control,
                      color: fleetColors.primary,
                      ml: 1,
                    }}
                  />
                )}
              </ListItemButton>
            );
          })}
        </List>
      </Popover>
    </>
  );
};
