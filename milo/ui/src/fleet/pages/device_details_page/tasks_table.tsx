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

import { StyledGrid } from '@/fleet/components/data_table/styled_data_grid';

interface Task {
  id: string;
  task: string;
  started: string;
  duration: string;
  result: string;
}

export const Tasks = () => {
  const mockTasks: Task[] = [
    {
      id: '1',
      task: 'bb-49187241987-chromeos/labpack_runner/repair',
      started: '10/20/2024, 1:37:12 AM (PDT)',
      duration: '1m 37s',
      result: 'SUCCESS',
    },
    {
      id: '2',
      task: 'bb-21784612874-chromeos/labpack_runner/repair',
      started: '10/20/2024, 1:28:25 AM (PDT)',
      duration: '1m 21s',
      result: 'SUCCESS',
    },
    {
      id: '3',
      task: 'bb-64785642387-chromeos/labpack_runner/repair',
      started: '10/20/2024, 1:27:05 AM (PDT)',
      duration: '1m 51s',
      result: 'SUCCESS',
    },
    {
      id: '4',
      task: 'bb-34194714192-chromeos/labpack_runner/repair',
      started: '10/20/2024, 1:16:52 AM (PDT)',
      duration: '2m 14s',
      result: 'SUCCESS',
    },
  ];

  const rows = mockTasks;
  const columns: GridColDef[] = [
    {
      field: 'task',
      headerName: 'Task',
      flex: 2,
    },
    {
      field: 'started',
      headerName: 'Started',
      flex: 1,
    },
    {
      field: 'duration',
      headerName: 'Duration',
      flex: 1,
    },
    {
      field: 'result',
      headerName: 'Result',
      flex: 1,
    },
  ];

  return (
    <StyledGrid
      rows={rows}
      columns={columns}
      getRowHeight={() => 'auto'}
      disableColumnMenu
      disableColumnFilter
      hideFooterPagination
    />
  );
};
