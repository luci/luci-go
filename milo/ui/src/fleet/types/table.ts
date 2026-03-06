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

import {
  MRT_Cell,
  MRT_Column,
  MRT_Row,
  MRT_RowData,
} from 'material-react-table';

export interface FC_CellProps<R extends MRT_RowData> {
  cell: MRT_Cell<R, unknown>;
  row: MRT_Row<R>;
  column: MRT_Column<R, unknown>;
}
