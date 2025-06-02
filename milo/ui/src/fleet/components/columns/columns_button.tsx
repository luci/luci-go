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

import Button from '@mui/material/Button';
import { GridColumnIcon } from '@mui/x-data-grid';
import { useState } from 'react';

import { ColumnsManageDropDown } from './column_manage_dropdown';

interface ColumnsButtonProps {
  isLoading?: boolean;
  defaultColumns: string[];
}

/**
 * Component for displaying a menu to customize the columns on a table.
 */
export function ColumnsButton({
  isLoading,
  defaultColumns,
}: ColumnsButtonProps) {
  const [anchorEl, setAnchorEL] = useState<HTMLElement | null>(null);

  return (
    <>
      <Button
        onClick={(event) => setAnchorEL(event.currentTarget)}
        size="small"
        startIcon={<GridColumnIcon />}
      >
        Columns
      </Button>
      <ColumnsManageDropDown
        isLoading={isLoading}
        anchorEl={anchorEl}
        setAnchorEL={setAnchorEL}
        defaultColumns={defaultColumns}
      />
    </>
  );
}
