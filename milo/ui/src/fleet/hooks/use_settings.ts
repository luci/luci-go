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
import { useLocalStorage } from 'react-use';

import { SETTINGS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';

export interface Settings {
  table: {
    density: GridDensity;
  };
  tableMRT: {
    density: MRT_DensityState;
  };
}

export const defaultSettings: Settings = {
  table: {
    density: 'compact',
  },
  tableMRT: {
    density: 'compact',
  },
};

export function useSettings(): [Settings, (value: Settings) => void] {
  const [settings, setSettings] = useLocalStorage<Settings>(
    SETTINGS_LOCAL_STORAGE_KEY,
    defaultSettings,
  );

  return [settings || defaultSettings, setSettings];
}
