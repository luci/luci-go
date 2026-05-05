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
  onKeyDown?: React.KeyboardEventHandler<HTMLDivElement>;
}

const StyledToggleButtonGroup = styled(ToggleButtonGroup, {
  shouldForwardProp: (prop) =>
    prop !== 'selectedIndex' && prop !== 'totalOptions',
})<{
  selectedIndex: number;
  totalOptions: number;
}>(({ selectedIndex, totalOptions }) => ({
  position: 'relative',
  borderRadius: '20px',
  border: 'none',
  overflow: 'hidden',
  backgroundColor: colors.grey[50],
  width: '100%',
  '&:focus-visible': {
    outline: `2px solid ${colors.blue[700]}`,
    outlineOffset: '2px',
  },
  '&::before': {
    content: '""',
    position: 'absolute',
    top: 0,
    left: 0,
    width: `${100 / totalOptions}%`,
    height: '100%',
    backgroundColor: colors.blue[50],
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

const StyledToggleButton = styled(ToggleButton)(() => ({
  transition: 'ease-in-out 0.2s all',
  padding: '2px 16px',
  fontSize: '14px',
  fontWeight: 500,
  textTransform: 'none',
  color: colors.grey[700],
  borderRadius: '20px',
  border: 'none',
  flex: 1,
  '&.Mui-selected': {
    backgroundColor: 'transparent',
    fontWeight: 500,
    color: colors.blue[700],
    borderRadius: '20px',
    '&:hover': {
      backgroundColor: colors.blue[100],
    },
  },
  '&:hover': {
    backgroundColor: colors.grey[100],
    borderRadius: '20px',
  },
}));

export const SegmentedToggle = ({
  options,
  value,
  onChange,
  onKeyDown,
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
      tabIndex={0}
      onKeyDown={(e) => {
        const isLeft =
          e.key === 'ArrowLeft' || (e.key === 'h' && (e.metaKey || e.ctrlKey));
        const isRight =
          e.key === 'ArrowRight' || (e.key === 'l' && (e.metaKey || e.ctrlKey));

        if (isLeft || isRight) {
          const currentIndex = options.findIndex((opt) => opt.value === value);
          let nextIndex = currentIndex;
          if (isLeft) {
            nextIndex = Math.max(0, currentIndex - 1);
          } else if (isRight) {
            nextIndex = Math.min(options.length - 1, currentIndex + 1);
          }
          onChange(options[nextIndex].value);

          e.preventDefault();
          e.stopPropagation();
        }

        if (e.key === ' ' || e.key === 'Enter') {
          const currentIndex = options.findIndex((opt) => opt.value === value);
          const nextIndex = (currentIndex + 1) % options.length;
          onChange(options[nextIndex].value);

          e.preventDefault();
          e.stopPropagation();
        }
        onKeyDown?.(e);
      }}
    >
      {options.map((option) => (
        <StyledToggleButton
          key={option.value}
          value={option.value}
          aria-label={option.label}
          tabIndex={-1}
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
