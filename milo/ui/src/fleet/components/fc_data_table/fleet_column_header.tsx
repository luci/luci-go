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

import { Box, Typography, useTheme } from '@mui/material';
import {
  MRT_Header,
  MRT_RowData,
  MRT_TableInstance,
  MRT_TableHeadCellSortLabel,
  MRT_TableHeadCellColumnActionsButton,
} from 'material-react-table';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { colors } from '@/fleet/theme/colors';

export interface FleetColumnHeaderProps<TData extends MRT_RowData> {
  column: MRT_Header<TData>['column'];
  header: MRT_Header<TData>;
  table: MRT_TableInstance<TData>;
  headerText?: string;
  infoTooltip?: React.ReactNode;
}

export const FleetColumnHeader = <TData extends MRT_RowData>({
  header,
  table,
  headerText,
  infoTooltip,
}: FleetColumnHeaderProps<TData>) => {
  const theme = useTheme();
  const { column } = header;
  const { columnDef } = column;
  const meta = columnDef.meta;

  const resolvedText = headerText ?? (columnDef.header as string) ?? column.id;
  const resolvedTooltip = infoTooltip ?? meta?.infoTooltip;

  const canSort = column.getCanSort();
  const canAction =
    (table.options.enableColumnActions ?? true) &&
    (column.columnDef.enableColumnActions ?? true);

  const isHighlighted = meta?.isHighlighted;

  // Derive line height and max height for line clamping (up to 2 lines)
  const typography = theme.typography.subhead2;
  const lineHeight = typography?.lineHeight ?? '1.2rem';
  const maxHeight =
    typeof lineHeight === 'number'
      ? `${lineHeight * 2}px`
      : `calc(${lineHeight} * 2)`;

  return (
    <Box
      className="fleet-column-header"
      sx={{
        display: 'flex',
        alignItems: 'center',
        width: '100%',
        minWidth: 0,
        flexShrink: 1,
        // Resetting pointerEvents here to let .styles handle it
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          flex: '1 1 0%',
          minWidth: 0,
          overflow: 'hidden',
        }}
      >
        <Typography
          variant="subhead2"
          title={typeof resolvedText === 'string' ? resolvedText : undefined}
          sx={{
            fontWeight: 500,
            color: isHighlighted ? colors.blue[600] : colors.grey[800],
            mr: 0.5,
            minWidth: 0,
            flex: '1 1 0%',
            wordBreak: 'break-word',
            overflowWrap: 'anywhere',
            whiteSpace: 'normal',
            maxHeight,
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
            lineHeight,
          }}
        >
          {resolvedText}
        </Typography>

        {resolvedTooltip && (
          <Box sx={{ ml: 0.5, mr: 0.5, display: 'flex', flexShrink: 0 }}>
            <InfoTooltip>{resolvedTooltip}</InfoTooltip>
          </Box>
        )}
      </Box>

      {/* Action Container behaves naturally - .styles control its appearance */}
      <Box
        className={`fleet-column-actions ${column.getIsSorted() ? 'is-sorted' : ''}`}
        onClick={(e) => e.stopPropagation()}
        sx={{
          alignItems: 'center',
          backgroundColor: isHighlighted
            ? `${colors.blue[50]}`
            : 'var(--header-bg-color)',
          pointerEvents: 'none',
          zIndex: 999,
          // Visibility logic is now fully centralized in fleetTableHeaderSx
        }}
      >
        {canSort && (
          <MRT_TableHeadCellSortLabel header={header} table={table} />
        )}
        {canAction && (
          <MRT_TableHeadCellColumnActionsButton header={header} table={table} />
        )}
      </Box>
    </Box>
  );
};
