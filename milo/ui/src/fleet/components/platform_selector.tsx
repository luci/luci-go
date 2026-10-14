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
import CheckIcon from '@mui/icons-material/Check';
import {
  Button,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Popover,
  Tooltip,
} from '@mui/material';
import _ from 'lodash';
import { useState } from 'react';

import { useFeatureFlag } from '@/common/feature_flags';
import androidLogo from '@/fleet/assets/logos/android.png';
import browserLogo from '@/fleet/assets/logos/browser.png';
import chromeosLogo from '@/fleet/assets/logos/chromeos.png';
import pixelLogo from '@/fleet/assets/logos/pixel.png';
import { usePlatform, platformRenderString } from '@/fleet/hooks/usePlatform';
import {
  borders,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { enablePTE } from '../features';

import { useShortcut } from './shortcut_provider';

const PLATFORM_LOGOS: Partial<Record<Platform, { src: string; alt: string }>> =
  {
    [Platform.CHROMEOS]: { src: chromeosLogo, alt: 'Chrome OS' },
    [Platform.CHROMIUM]: { src: browserLogo, alt: 'Chrome Browser' },
    [Platform.ANDROID]: { src: androidLogo, alt: 'Android' },
    [Platform.PIXEL]: { src: pixelLogo, alt: 'Pixel' },
  };

function PlatformLogo({ platform }: { platform?: Platform }) {
  const logo =
    (platform !== undefined && PLATFORM_LOGOS[platform]) ||
    PLATFORM_LOGOS[Platform.CHROMEOS]!;
  return (
    <img
      src={logo.src}
      alt={logo.alt}
      style={{
        width: iconSizes.logo,
        height: iconSizes.logo,
        objectFit: 'contain',
        display: 'block',
        flexShrink: 0,
      }}
    />
  );
}

export function PlatformSelector() {
  const platform = usePlatform();
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);

  const PLATFORMS: Platform[] = [
    Platform.CHROMEOS,
    Platform.CHROMIUM,
    Platform.ANDROID,
    ...(useFeatureFlag(enablePTE) ? [Platform.PIXEL] : []),
  ];

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

  useShortcut(
    'Switch to Android',
    'g a',
    () => handlePlatformSelect(Platform.ANDROID),
    { category: 'Navigation' },
  );
  useShortcut(
    'Switch to Chrome OS',
    'g c',
    () => handlePlatformSelect(Platform.CHROMEOS),
    { category: 'Navigation' },
  );
  useShortcut(
    'Switch to Chrome Browser',
    'g b',
    () => handlePlatformSelect(Platform.CHROMIUM),
    { category: 'Navigation' },
  );
  useShortcut(
    'Switch to Pixel',
    'g x',
    () => handlePlatformSelect(Platform.PIXEL),
    { category: 'Navigation', enabled: useFeatureFlag(enablePTE) },
  );

  return (
    <>
      <Tooltip title="Switch platform">
        <Button
          variant="toolbar"
          onClick={handleClick}
          startIcon={<PlatformLogo platform={platform.platform} />}
          endIcon={<ArrowDropDownIcon />}
          sx={{ ml: 0.5 }}
        >
          {platformRenderString(platform.platform) || 'Invalid platform'}
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
            sx: { mt: 0.75, border: borders.default, minWidth: 196 },
          },
        }}
      >
        <List sx={{ p: 0.75 }}>
          {PLATFORMS.map((p) => {
            const selected = p === platform.platform;
            return (
              <ListItemButton
                key={p}
                selected={selected}
                onClick={() => handlePlatformSelect(p)}
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
                  <PlatformLogo platform={p} />
                </ListItemIcon>
                <ListItemText
                  primary={platformRenderString(p)}
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
}
