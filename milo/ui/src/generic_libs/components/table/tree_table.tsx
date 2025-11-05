import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  Box,
  Typography,
  Paper,
  SxProps,
  Theme,
  useTheme,
} from '@mui/material';
import React, { useState, useCallback, useMemo, ReactNode } from 'react';

export interface RowData {
  id: string;
  children?: RowData[];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  [key: string]: any;
}

export interface ColumnDefinition {
  id: string;
  label: string;
  width?: string | number;
  minWidth?: string | number;
  align?: 'left' | 'right' | 'center' | 'inherit' | 'justify';
  renderCell?: (row: RowData, theme: Theme) => ReactNode;
}

interface VisibleRow extends RowData {
  level: number;
}

interface TreeTableProps {
  data: RowData[];
  columns: ColumnDefinition[];
  // Optional props to control expansion state from the parent.  If not provided the
  // TreeTable will manage its own expansion state.
  expandedRowIds?: Set<string>;
  onExpandedRowIdsChange?: (newExpandedIds: Set<string>) => void;
  // React nodes to put in a TableRow/TableCell above the column headers.
  // Intended for filter bar or similar.
  headerChildren: React.ReactNode;
  placeholder: string;
}

const getVisibleRows = (
  nodes: RowData[] | undefined,
  expandedRowIds: Set<string>,
  level: number = 0,
): VisibleRow[] => {
  let visibleRows: VisibleRow[] = [];
  if (!nodes) return visibleRows;
  nodes.forEach((node) => {
    visibleRows.push({ ...node, level });
    if (
      node.children &&
      node.children.length > 0 &&
      expandedRowIds.has(node.id)
    ) {
      visibleRows = visibleRows.concat(
        getVisibleRows(node.children, expandedRowIds, level + 1),
      );
    }
  });
  return visibleRows;
};

/**
 * A tree table component capable of displaying hierarchical data.
 * It supports custom column rendering and managing expanded rows.
 * The expansion state can be controlled externally via `expandedRowIds` and `onExpandedRowIdsChange` props.
 */
export function TreeTable({
  data,
  columns,
  expandedRowIds: expandedRowIdsFromProps,
  onExpandedRowIdsChange,
  headerChildren,
  placeholder,
}: TreeTableProps) {
  const theme = useTheme();

  // The component's internal state for when it's uncontrolled.
  const [internalExpandedIds, setInternalExpandedIds] = useState<Set<string>>(
    new Set(),
  );

  // Determine if the component is in controlled mode.
  const isControlled = expandedRowIdsFromProps !== undefined;

  // Use the appropriate set of IDs (from props if controlled, from state if not).
  const expandedRowIds = isControlled
    ? expandedRowIdsFromProps
    : internalExpandedIds;

  const handleToggleExpand = useCallback(
    (rowId: string) => {
      const currentIds = isControlled
        ? expandedRowIdsFromProps
        : internalExpandedIds;
      const newExpanded = new Set(currentIds);
      if (newExpanded.has(rowId)) {
        newExpanded.delete(rowId);
      } else {
        newExpanded.add(rowId);
      }

      if (isControlled) {
        onExpandedRowIdsChange?.(newExpanded);
      } else {
        setInternalExpandedIds(newExpanded);
      }
    },
    [
      isControlled,
      expandedRowIdsFromProps,
      internalExpandedIds,
      onExpandedRowIdsChange,
    ],
  );

  const visibleRows = useMemo(
    () => getVisibleRows(data, expandedRowIds),
    [data, expandedRowIds],
  );

  const bodyCellSx: SxProps<Theme> = {
    ...theme.typography.body2,
    borderBottom: `1px solid ${theme.palette.divider}`,
    minHeight: '36px',
    paddingTop: '6px',
    paddingBottom: '6px',
  };

  return (
    <TableContainer
      component={Paper}
      sx={{
        borderRadius: '8px',
        border: `1px solid ${theme.palette.divider}`,
        backgroundColor: theme.palette.background.paper,
        borderCollapse: 'collapse',
      }}
    >
      <Table stickyHeader aria-label="tree table" size="small">
        <TableHead>
          <TableRow
            sx={{
              minHeight: '36px',
            }}
          >
            <TableCell colSpan={columns.length}>{headerChildren}</TableCell>
          </TableRow>
          <TableRow
            sx={{
              minHeight: '36px',
            }}
          >
            {columns.map((column) => (
              <TableCell
                key={column.id}
                align={column.align || 'left'}
                style={{
                  minWidth: column.minWidth || column.width || 'auto',
                  width: column.width || 'auto',
                }}
                sx={{
                  ...theme.typography.subtitle2,
                  borderTop: `1px solid ${theme.palette.divider}`,
                  borderBottom: `1px solid ${theme.palette.divider}`,
                  borderCollapse: 'collapse',
                  minHeight: '36px',
                  paddingTop: '6px',
                  paddingBottom: '6px',
                }}
              >
                {column.label}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {visibleRows.map((row) => (
            <TableRow
              key={row.id}
              hover
              sx={{
                minHeight: '36px',
                '&:last-child td, &:last-child th': { border: 0 },
              }}
            >
              {columns.map((column, colIndex) => {
                const cellContent = row[column.id];
                return (
                  <TableCell
                    key={`${row.id}-${column.id}-col`}
                    align={column.align || 'left'}
                    sx={bodyCellSx}
                  >
                    {colIndex === 0 ? (
                      <Box
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          paddingLeft: `${row.level * 24}px`,
                        }}
                      >
                        {row.children && row.children.length > 0 ? (
                          <IconButton
                            aria-label={
                              expandedRowIds.has(row.id)
                                ? 'collapse row'
                                : 'expand row'
                            }
                            size="small"
                            onClick={() => handleToggleExpand(row.id)}
                            sx={{
                              padding: 0,
                              color: theme.palette.gm3.onSurfaceVariant,
                            }}
                          >
                            {expandedRowIds.has(row.id) ? (
                              <ExpandMoreIcon />
                            ) : (
                              <ChevronRightIcon />
                            )}
                          </IconButton>
                        ) : (
                          <Box sx={{ width: '28px', height: '28px' }} />
                        )}
                        <Typography
                          variant="body2"
                          component="span"
                          sx={{
                            marginLeft:
                              row.children && row.children.length > 0
                                ? '8px'
                                : '0px',
                            color: 'inherit',
                          }}
                        >
                          {column.renderCell
                            ? column.renderCell(row, theme)
                            : cellContent}
                        </Typography>
                      </Box>
                    ) : column.renderCell ? (
                      column.renderCell(row, theme)
                    ) : (
                      cellContent
                    )}
                  </TableCell>
                );
              })}
            </TableRow>
          ))}
          {visibleRows.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={columns.length}
                align="center"
                sx={{
                  ...bodyCellSx,
                  color: theme.palette.gm3.onSurfaceVariant,
                }}
              >
                <Typography variant="body2" sx={{ padding: '20px' }}>
                  {placeholder || 'No data to display.'}
                </Typography>
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
