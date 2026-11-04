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

import CheckIcon from '@mui/icons-material/Check';
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import { Box, Button, Tooltip } from '@mui/material';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_Row,
} from 'material-react-table';
import { useMemo } from 'react';

import { fleetTableHeaderSx } from '@/fleet/components/fc_data_table/fleet_column_header.styles';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';

import {
  GCE_ONLY_ACCESSORS,
  HARDWARE_ONLY_ACCESSORS,
  resolveProductType,
} from './catalog_details_helpers';
import {
  CatalogEditableFieldKey,
  CatalogEditableFields,
  EDITABLE_SPEC_FIELD_BY_HEADER,
} from './catalog_edit_validation';
import { DetailRow, DetailValueCell } from './detail_value_cell';
import { COLUMNS } from './product_catalogue_columns';
import { UnifiedProductCatalogEntry } from './types';

export interface CatalogSpecsTableProps {
  readonly entry?: UnifiedProductCatalogEntry;
  readonly displayEntry?: UnifiedProductCatalogEntry;
  readonly isRevampEnabled: boolean;
  readonly isEditingSpecs: boolean;
  readonly onToggleEditingSpecs: () => void;
  readonly onStartEditingSpecs: () => void;
  readonly onStopEditingSpecs?: () => void;
  readonly activeSpecDraft: CatalogEditableFields;
  readonly baselineSpecDraft: CatalogEditableFields;
  readonly specErrors: Partial<Record<CatalogEditableFieldKey, string>>;
  readonly onSpecFieldChange: (
    field: CatalogEditableFieldKey,
    value: string,
  ) => void;
}

const REVAMP_TABLE_SX = {
  tableLayout: 'fixed',
  width: '100%',
  '&& .MuiTableCell-root:first-of-type': {
    width: 210,
    minWidth: 210,
    maxWidth: 210,
    paddingLeft: '16px',
    paddingRight: '12px',
    boxSizing: 'border-box',
  },
  '&& .MuiTableCell-root:last-of-type': {
    width: 'auto',
    minWidth: 0,
    paddingLeft: '16px',
    paddingRight: '16px',
    boxSizing: 'border-box',
  },
} as const;

export const CatalogSpecsTable = ({
  entry,
  displayEntry,
  isRevampEnabled,
  isEditingSpecs,
  onToggleEditingSpecs,
  onStartEditingSpecs,
  onStopEditingSpecs,
  activeSpecDraft,
  baselineSpecDraft,
  specErrors,
  onSpecFieldChange,
}: CatalogSpecsTableProps) => {
  const activeEntry = displayEntry ?? entry;
  const resolvedProductType = resolveProductType(
    activeEntry?.productType,
    activeEntry?.productCatalogId,
  );
  const isGceEntry = resolvedProductType === 'gce';

  const columns = useMemo<MRT_ColumnDef<DetailRow>[]>(
    () => [
      {
        accessorKey: 'key',
        header: 'Field',
        size: 210,
        minSize: 210,
        maxSize: 210,
        grow: 0,
        muiTableBodyCellProps: {
          sx: {
            fontWeight: 500,
            color: 'text.secondary',
            ...(isRevampEnabled
              ? {
                  py: 0,
                  height: 44,
                  minHeight: 44,
                  maxHeight: 44,
                  boxSizing: 'border-box',
                  verticalAlign: 'middle',
                }
              : {}),
          },
        },
      },
      {
        accessorKey: 'rawValue',
        header: 'Value',
        size: 240,
        grow: 1,
        muiTableHeadCellProps:
          isRevampEnabled && !isGceEntry
            ? {
                'aria-label': 'Value',
                sx: {
                  ...fleetTableHeaderSx,
                  py: 0.5,
                  '& .Mui-TableHeadCell-Content, & .Mui-TableHeadCell-Content-Labels, & .Mui-TableHeadCell-Content-Wrapper':
                    { width: '100%' },
                },
              }
            : undefined,
        Header:
          isRevampEnabled && !isGceEntry
            ? () => {
                const editLabel = isEditingSpecs
                  ? 'Done editing specifications'
                  : 'Edit specifications';
                return (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      width: '100%',
                      gap: 1,
                    }}
                  >
                    <span>Value</span>
                    <Tooltip title={editLabel}>
                      <Button
                        variant={isEditingSpecs ? 'contained' : 'outlined'}
                        color="primary"
                        size="small"
                        aria-label={editLabel}
                        onClick={onToggleEditingSpecs}
                        data-testid="toggle-spec-editor-btn"
                        startIcon={
                          isEditingSpecs ? <CheckIcon /> : <EditOutlinedIcon />
                        }
                      >
                        {isEditingSpecs ? 'Done' : 'Edit'}
                      </Button>
                    </Tooltip>
                  </Box>
                );
              }
            : undefined,
        muiTableBodyCellProps: {
          sx: {
            ...(isRevampEnabled
              ? {
                  minWidth: 0,
                  py: 0,
                  height: 44,
                  minHeight: 44,
                  maxHeight: 44,
                  boxSizing: 'border-box',
                  verticalAlign: 'middle',
                }
              : {}),
          },
        },
        Cell: ({ row }: { readonly row: MRT_Row<DetailRow> }) => (
          <DetailValueCell
            row={row}
            isGce={isGceEntry}
            isRevampEnabled={isRevampEnabled}
            isEditingSpecs={isEditingSpecs}
            onStartEditingSpecs={onStartEditingSpecs}
            onStopEditingSpecs={onStopEditingSpecs}
            onSpecFieldChange={onSpecFieldChange}
          />
        ),
      },
    ],
    [
      isEditingSpecs,
      isGceEntry,
      isRevampEnabled,
      onSpecFieldChange,
      onStartEditingSpecs,
      onStopEditingSpecs,
      onToggleEditingSpecs,
    ],
  );

  const data = useMemo<DetailRow[]>(() => {
    if (!activeEntry) return [];
    const filteredCols = isRevampEnabled
      ? COLUMNS.filter((col) => {
          const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;
          return isGceEntry
            ? !HARDWARE_ONLY_ACCESSORS.has(accessor)
            : !GCE_ONLY_ACCESSORS.has(accessor);
        })
      : COLUMNS;

    return filteredCols.map((col) => {
      const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;
      const header = col.header as string;
      const editableKey = EDITABLE_SPEC_FIELD_BY_HEADER[header];
      const val =
        isRevampEnabled && accessor === 'productType'
          ? activeEntry.productType || resolvedProductType
          : activeEntry[accessor];
      return {
        key: header,
        rawValue: val,
        draftValue: editableKey ? activeSpecDraft[editableKey] : undefined,
        baselineValue: editableKey ? baselineSpecDraft[editableKey] : undefined,
        errorMsg: editableKey ? specErrors[editableKey] : undefined,
      };
    });
  }, [
    activeEntry,
    activeSpecDraft,
    baselineSpecDraft,
    isGceEntry,
    isRevampEnabled,
    resolvedProductType,
    specErrors,
  ]);

  const table = useFCDataTable({
    columns,
    data,
    layoutMode: isRevampEnabled ? 'semantic' : 'grid',
    enableRowVirtualization: false,
    enablePagination: false,
    enableColumnActions: false,
    enableColumnResizing: !isRevampEnabled,
    enableSorting: false,
    enableTopToolbar: false,
    enableBottomToolbar: false,
    enableTableHead: true,
    enableStickyHeader: true,
    muiTableProps: isRevampEnabled ? { sx: REVAMP_TABLE_SX } : undefined,
    muiTablePaperProps: isRevampEnabled
      ? {
          elevation: 0,
          sx: { border: 'none', borderRadius: 0, boxShadow: 'none' },
        }
      : undefined,
    muiTableContainerProps: {
      sx: {
        maxWidth: '100%',
        overflowX: 'hidden',
        margin: isRevampEnabled ? 0 : '24px 0',
        ...(isRevampEnabled ? { maxHeight: 'none', overflowY: 'visible' } : {}),
      },
    },
  });

  return <MaterialReactTable table={table} />;
};
