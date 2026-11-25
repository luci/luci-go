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

import DarkModeOutlined from '@mui/icons-material/DarkModeOutlined';
import LightModeOutlined from '@mui/icons-material/LightModeOutlined';
import { IconButton, Tooltip } from '@mui/material';

import { useFleetTheme } from './fleet_theme_context';

/**
 * Top-level header icon button for 1-click Light <-> Dark toggling.
 * The full Light / Dark / Auto picker remains available in `SettingsMenu`.
 */
export function ThemeToggleButton() {
  const fleetTheme = useFleetTheme();
  if (!fleetTheme) return null;

  const isDark = fleetTheme.palette.mode === 'dark';
  const nextChoice = isDark ? 'light' : 'dark';
  const label = isDark ? 'Switch to light mode' : 'Switch to dark mode';

  return (
    <Tooltip title={label}>
      <IconButton
        onClick={() => fleetTheme.setChoice(nextChoice, 'header_toggle')}
        aria-label={label}
      >
        {isDark ? <LightModeOutlined /> : <DarkModeOutlined />}
      </IconButton>
    </Tooltip>
  );
}
