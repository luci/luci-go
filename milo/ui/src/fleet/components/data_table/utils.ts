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

import { GridColDef } from '@mui/x-data-grid';

export function generateColDefs(
  columns: string[],
  columnHeaders?: { [key: string]: string },
): GridColDef[] {
  const colDefs: GridColDef[] = columns.map((column) => ({
    field: column,
    headerName: columnHeaders?.[column] ?? column,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
  }));
  return colDefs;
}
