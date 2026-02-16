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

import { GridDensity } from '@mui/x-data-grid';
import { MRT_DensityState } from 'material-react-table';
import { useContext } from 'react';

import { SettingsContext } from '../context/providers';

export const mapMUIToMRT = (density: GridDensity): MRT_DensityState => {
  switch (density) {
    case 'compact':
      return 'compact';
    case 'standard':
      return 'comfortable';
    case 'comfortable':
      return 'spacious';
  }
};

export const mapMRTToMUI = (density: MRT_DensityState): GridDensity => {
  switch (density) {
    case 'compact':
      return 'compact';
    case 'comfortable':
      return 'standard';
    case 'spacious':
      return 'comfortable';
  }
};

export function useSettings() {
  const context = useContext(SettingsContext);
  if (!context)
    throw new Error('useSettings must be used within SettingsProvider');

  const [settings, setSettings] = context;

  return [settings, setSettings] as const;
}
