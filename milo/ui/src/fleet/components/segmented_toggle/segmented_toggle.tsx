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

import { ToggleButtonGroup, ToggleButton, styled } from '@mui/material';
import React from 'react';

import { colors } from '@/fleet/theme/colors';

export interface ToggleOption {
  value: string;
  label: string;
  icon?: React.ReactNode;
}

interface SegmentedToggleProps {
  options: ToggleOption[];
  value: string;
  onChange: (newValue: string) => void;
}

const StyledToggleButtonGroup = styled(ToggleButtonGroup)<{
  selectedIndex: number;
  totalOptions: number;
}>(({ selectedIndex, totalOptions, theme }) => ({
  position: 'relative',
  borderRadius: '20px',
  border: 'none',
  overflow: 'hidden',
  backgroundColor: colors.blue[50],
  width: '100%',
  '&::before': {
    content: '""',
    position: 'absolute',
    top: 0,
    left: 0,
    width: `${100 / totalOptions}%`,
    height: '100%',
    backgroundColor: theme.palette.primary.main,
    borderRadius: '20px',
    transition: 'transform 0.1s ease-in-out',
    transform: `translateX(${selectedIndex * 100}%)`,
    zIndex: 0,
  },
  '& .MuiToggleButtonGroup-grouped': {
    margin: 0,
    border: 0,
    borderRadius: '20px',
    zIndex: 1,
    '&.Mui-disabled': {
      border: 0,
    },
  },
}));

const StyledToggleButton = styled(ToggleButton)(({ theme }) => ({
  transition: 'ease-in-out 0.2s all',
  padding: '2px 16px',
  fontSize: '14px',
  fontWeight: 500,
  textTransform: 'none',
  color: colors.blue[800],
  borderRadius: '20px',
  border: 'none',
  flex: 1,
  '&.Mui-selected': {
    backgroundColor: 'transparent',
    color: theme.palette.primary.contrastText,
    borderRadius: '20px',
    '&:hover': {
      backgroundColor: 'transparent',
    },
  },
  '&:hover': {
    backgroundColor: colors.blue[100],
    borderRadius: '20px',
  },
}));

export const SegmentedToggle = ({
  options,
  value,
  onChange,
}: SegmentedToggleProps) => {
  const handleChange = (
    _event: React.MouseEvent<HTMLElement>,
    newValue: string | null,
  ) => {
    if (newValue !== null) {
      onChange(newValue);
    }
  };

  const selectedIndex = options.findIndex((option) => option.value === value);

  return (
    <StyledToggleButtonGroup
      value={value}
      exclusive
      onChange={handleChange}
      aria-label="segmented toggle"
      size="small"
      selectedIndex={selectedIndex}
      totalOptions={options.length}
    >
      {options.map((option) => (
        <StyledToggleButton
          key={option.value}
          value={option.value}
          aria-label={option.label}
        >
          {option.icon && (
            <span
              style={{
                marginRight: '4px',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              {option.icon}
            </span>
          )}
          {option.label}
        </StyledToggleButton>
      ))}
    </StyledToggleButtonGroup>
  );
};
