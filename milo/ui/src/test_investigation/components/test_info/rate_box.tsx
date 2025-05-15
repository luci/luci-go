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

import { Box, Typography } from '@mui/material';
import { JSX } from 'react';

import { getStatusStyle, StatusStyle } from '@/common/styles/status_styles';
import { RateBoxDisplayInfo } from '@/test_investigation/utils/test_info_utils';

interface RateBoxProps {
  info: RateBoxDisplayInfo;
}

export function RateBox({ info }: RateBoxProps): JSX.Element {
  const style: StatusStyle = getStatusStyle(info.statusType);
  const IconComponent = style.icon;

  return (
    <Box
      sx={{
        backgroundColor: style.backgroundColor,
        color: style.textColor,
        padding: '2px 8px',
        borderRadius: '4px',
        display: 'inline-flex',
        alignItems: 'center',
        gap: 0.5,
        fontSize: '0.875rem',
        // Apply border if borderColor is defined in the style
        borderStyle: style.borderColor ? 'solid' : 'none',
        borderWidth: style.borderColor ? '1px' : '0',
        borderColor: style.borderColor || 'transparent', // Default to transparent if not set
      }}
      aria-label={info.ariaLabel}
    >
      {IconComponent && (
        <IconComponent
          sx={{
            fontSize: 16,
            color: style.iconColor || 'inherit',
          }}
        />
      )}
      <Typography
        variant="body2"
        component="span"
        sx={{
          fontWeight: 'bold',
          color: 'inherit',
        }}
      >
        {info.text}
      </Typography>
    </Box>
  );
}
