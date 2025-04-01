// Copyright 2024 The LUCI Authors.
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

import { colors } from '@mui/material';
import {
  DataGrid,
  DataGridProps,
  gridClasses,
  GridDensity,
} from '@mui/x-data-grid';

import { useSettings } from '@/fleet/hooks/use_settings';

import { TableToolbar } from '../toolbar/table_toolbar';

export const StyledGrid = ({ sx, ...props }: DataGridProps) => {
  const [settings, setSettings] = useSettings();

  if (!props.slots?.toolbar) {
    props.slots = props.slots ?? {};
    props.slots.toolbar = TableToolbar;
  }

  return (
    <DataGrid
      density={settings.table.density}
      onDensityChange={(density: GridDensity) => {
        settings.table.density = density;
        setSettings(settings);
      }}
      columnHeaderHeight={settings.table.density === 'compact' ? 75 : 56}
      sx={{
        border: 'none',
        [`& .${gridClasses.columnHeader}`]: {
          backgroundColor: colors.grey[100],
        },
        [`& .${gridClasses.columnSeparator}`]: {
          color: colors.grey[400],
        },
        [`& .${gridClasses.filler}`]: {
          background: colors.grey[100],
        },
        [`& .${gridClasses.columnHeaderTitle}`]: {
          whiteSpace: 'normal',
        },
        ...sx,
      }}
      {...props}
    />
  );
};
