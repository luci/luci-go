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

import Chip from '@mui/material/Chip';

import { useSettings } from '@/fleet/hooks/use_settings';
import { colors } from '@/fleet/theme/colors';
import {
  borders,
  controlHeights,
  fleetColors,
  fontSizes,
  statusTones,
} from '@/fleet/theme/tokens';

export interface ChipComponentProps {
  label: string;
  color?: string;
  url?: string;
  openInNewTab?: boolean;
  onClick?: () => void;
}

const getSemanticTextColor = (bgColor?: string): string => {
  if (!bgColor || bgColor === colors.transparent)
    return fleetColors.textSecondary;
  if (bgColor === colors.green[100] || bgColor === colors.green[50]) {
    return statusTones.success.fg;
  }
  if (
    bgColor === colors.orange[100] ||
    bgColor === colors.yellow[100] ||
    bgColor === colors.orange[50] ||
    bgColor === colors.yellow[50]
  ) {
    return statusTones.warning.fg;
  }
  if (bgColor === colors.red[100] || bgColor === colors.red[50]) {
    return statusTones.error.fg;
  }
  if (bgColor === colors.purple[100] || bgColor === colors.purple[50]) {
    return statusTones.special.fg;
  }
  if (bgColor === colors.blue[100] || bgColor === colors.blue[50]) {
    return statusTones.info.fg;
  }
  return fleetColors.textPrimary;
};

export const ChipComponent = ({
  label,
  color,
  url,
  openInNewTab = true,
  onClick,
}: ChipComponentProps) => {
  const [settings] = useSettings();
  const density = settings?.table?.density;

  const variant =
    !color || color === colors.transparent ? 'outlined' : 'filled';

  return (
    <Chip
      label={label}
      variant={variant}
      size={density === 'compact' ? 'small' : 'medium'}
      sx={{
        backgroundColor: color,
        color: getSemanticTextColor(color),
        border:
          variant === 'filled' ? '1px solid transparent' : borders.default,
        height:
          density === 'compact'
            ? controlHeights.chipCompact
            : controlHeights.chip,
        width: 'fit-content',
        fontSize: fontSizes.caption,
        fontWeight: 500,
        '& .MuiChip-label': {
          px: 1.25,
        },
      }}
      href={url}
      target={url ? (openInNewTab ? '_blank' : '_self') : undefined}
      rel={url && openInNewTab ? 'noopener noreferrer' : undefined}
      component={url ? 'a' : 'div'}
      clickable={Boolean(url || onClick)}
      onClick={onClick}
    />
  );
};
