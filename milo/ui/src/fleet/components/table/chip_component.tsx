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
import {
  borders,
  controlHeights,
  fleetColors,
  fontSizes,
  statusTones,
  toneColors,
  transparent,
} from '@/fleet/theme/tokens';

export interface ChipComponentProps {
  label: string;
  color?: string;
  url?: string;
  openInNewTab?: boolean;
  onClick?: () => void;
}

const getSemanticTextColor = (bgColor?: string): string => {
  if (!bgColor || bgColor === transparent) return fleetColors.textSecondary;
  if (
    bgColor === toneColors.success.container ||
    bgColor === toneColors.success.surface
  ) {
    return statusTones.success.fg;
  }
  if (
    bgColor === toneColors.caution.container ||
    bgColor === toneColors.caution.surface
  ) {
    return toneColors.caution.onContainer;
  }
  if (
    bgColor === toneColors.warning.container ||
    bgColor === toneColors.warning.surface
  ) {
    return statusTones.warning.fg;
  }
  if (
    bgColor === toneColors.error.container ||
    bgColor === toneColors.error.surface
  ) {
    return statusTones.error.fg;
  }
  if (
    bgColor === toneColors.special.container ||
    bgColor === toneColors.special.surface
  ) {
    return statusTones.special.fg;
  }
  if (
    bgColor === toneColors.info.container ||
    bgColor === toneColors.info.surface
  ) {
    return statusTones.info.fg;
  }
  if (bgColor === fleetColors.unknownState) {
    return fleetColors.onUnknownState;
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

  const variant = !color || color === transparent ? 'outlined' : 'filled';

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
