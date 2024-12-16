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

import Box from '@mui/material/Box';
import {
  GridSlots,
  GridToolbarColumnsButton,
  GridToolbarContainer,
  PropsFromSlot,
} from '@mui/x-data-grid';

declare module '@mui/x-data-grid' {
  interface ToolbarPropsOverrides {
    setColumnsButtonEl: (element: HTMLButtonElement | null) => void;
  }
}

export function Toolbar({
  setColumnsButtonEl,
}: PropsFromSlot<GridSlots['toolbar']>) {
  return (
    <GridToolbarContainer>
      <Box sx={{ flexGrow: 1 }} />
      <GridToolbarColumnsButton ref={setColumnsButtonEl} />
    </GridToolbarContainer>
  );
}
