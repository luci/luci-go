// Copyright 2023 The LUCI Authors.
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

import { createTheme } from '@mui/material';

export const theme = createTheme({
  palette: {
    scheduled: {
      main: '#73808c',
    },
    started: {
      main: '#ff8000',
    },
    success: {
      main: '#169c16',
    },
    error: {
      main: '#d23a2d',
    },
    criticalFailure: {
      main: '#6c40bf',
    },
    canceled: {
      main: '#0084ff',
    },
  },
});

declare module '@mui/material/styles' {
  interface Palette {
    scheduled: Palette['primary'];
    started: Palette['primary'];
    success: Palette['primary'];
    criticalFailure: Palette['primary'];
    canceled: Palette['primary'];
  }

  interface PaletteOptions {
    scheduled: PaletteOptions['primary'];
    started: PaletteOptions['primary'];
    success: PaletteOptions['primary'];
    criticalFailure: PaletteOptions['primary'];
    canceled: PaletteOptions['primary'];
  }
}

declare module '@mui/material' {
  interface LinearProgressPropsColorOverrides {
    scheduled: true;
    started: true;
    success: true;
    criticalFailure: true;
    canceled: true;
  }
}
