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

import AddIcon from '@mui/icons-material/Add';
import BugReportOutlinedIcon from '@mui/icons-material/BugReportOutlined';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import { useCallback, useState } from 'react';

import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import {
  FilterCategory,
  FilterCategoryBuilder,
} from '@/fleet/components/filters/use_filters';
import { borders } from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';

import { IrmFilterDialog } from './irm_filter_dialog';
import { getBugUrl, isAllDevicesFilter } from './utils';

/**
 * The fields of an IRM incident rendered by the table. Structurally
 * compatible with the per-platform IRM incident protos.
 */
export interface RepairIrmIncident {
  readonly id: string;
  readonly masterBugId?: string;
  readonly title?: string;
  readonly expressionsAip160?: readonly string[];
  readonly affectedDevicesCount?: number;
}

export interface ActiveIrmTableProps {
  readonly incidents: readonly RepairIrmIncident[];
  readonly isLoading: boolean;
  readonly isError: boolean;
  readonly error: unknown;
  readonly filterBuilders?: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading?: boolean;
  readonly canEdit?: boolean;
  readonly isSyncingIncident?: (incidentId: string) => boolean;
  readonly updateIncident?: (req: {
    readonly id: string;
    readonly expressionsAip160: readonly string[];
  }) => Promise<unknown>;
  readonly searchPlaceholder?: string;
}

const EMPTY_FILTER_BUILDERS: Record<
  string,
  FilterCategoryBuilder<FilterCategory>
> = {};

interface ActiveIrmRowProps {
  readonly incident: RepairIrmIncident;
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly canEdit: boolean;
  readonly isSyncing: boolean;
  readonly searchPlaceholder: string;
  readonly onUpdateExpressions: (
    incidentId: string,
    nextExpressions: readonly string[],
  ) => void;
}

const ActiveIrmRow = ({
  incident,
  filterBuilders,
  isBuildersLoading,
  canEdit,
  isSyncing,
  searchPlaceholder,
  onUpdateExpressions,
}: ActiveIrmRowProps) => {
  const bugId = incident.masterBugId?.trim() ?? '';
  const title = incident.title?.trim() || '—';
  const rawExpressions = incident.expressionsAip160 ?? [];
  const isAllDevices = isAllDevicesFilter(rawExpressions);
  const expressions = isAllDevices
    ? []
    : rawExpressions.map((e) => e.trim()).filter(Boolean);
  const affectedCount = incident.affectedDevicesCount ?? 0;

  const [isDialogOpen, setIsDialogOpen] = useState(false);

  const handleOpenDialog = useCallback(() => {
    if (isSyncing) return;
    setIsDialogOpen(true);
  }, [isSyncing]);

  const handleCloseDialog = useCallback(() => {
    setIsDialogOpen(false);
  }, []);

  const handleSaveDialog = useCallback(
    (savedExpressions: readonly string[]) => {
      setIsDialogOpen(false);
      onUpdateExpressions(incident.id, savedExpressions);
    },
    [incident.id, onUpdateExpressions],
  );

  const filterText = isAllDevices
    ? 'All devices'
    : expressions.length > 0
      ? `${expressions.length} applied`
      : null;

  const filterTooltip = isAllDevices ? (
    'Matches all devices across the fleet'
  ) : expressions.length > 0 ? (
    <Box sx={{ p: 0.5 }}>
      {expressions.map((expr, i) => (
        <Typography
          key={i}
          variant="caption"
          component="div"
          sx={{ fontFamily: 'monospace' }}
        >
          {expr}
        </Typography>
      ))}
    </Box>
  ) : null;

  return (
    <TableRow hover>
      <TableCell sx={{ verticalAlign: 'middle', whiteSpace: 'nowrap' }}>
        {bugId ? (
          <Chip
            component="a"
            href={getBugUrl(bugId)}
            target="_blank"
            rel="noopener noreferrer"
            icon={<BugReportOutlinedIcon />}
            label={'b/' + bugId}
            clickable
            size="small"
            color="error"
            variant="outlined"
            sx={{ fontWeight: 600 }}
            data-testid="irm-chip"
          />
        ) : (
          <Typography variant="caption" color="text.secondary">
            N/A
          </Typography>
        )}
      </TableCell>
      <TableCell
        sx={{
          verticalAlign: 'middle',
          maxWidth: 0,
          overflow: 'hidden',
        }}
      >
        <EllipsisTooltip tooltip={title}>
          <Typography variant="body2" noWrap>
            {title}
          </Typography>
        </EllipsisTooltip>
      </TableCell>
      <TableCell
        sx={{
          verticalAlign: 'middle',
          whiteSpace: 'nowrap',
        }}
        data-testid="irm-filters-cell"
      >
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            minWidth: 0,
          }}
        >
          {filterText || canEdit ? (
            <Tooltip
              title={filterTooltip}
              arrow
              placement="top"
              disableInteractive
            >
              <span>
                <Button
                  variant="text"
                  disableRipple
                  disabled={isSyncing}
                  onClick={!isSyncing ? handleOpenDialog : undefined}
                  data-testid="irm-filters-btn"
                  sx={{
                    p: 0,
                    minWidth: 0,
                    whiteSpace: 'nowrap',
                    textTransform: 'none',
                    fontWeight: filterText ? 600 : 400,
                    fontSize: '0.875rem',
                    color: canEdit ? 'primary.main' : 'text.primary',
                    cursor: !isSyncing ? 'pointer' : 'default',
                    '&:hover': {
                      backgroundColor: 'transparent',
                      textDecoration: !isSyncing ? 'underline' : 'none',
                    },
                    '&.Mui-disabled': {
                      color: 'text.disabled',
                    },
                  }}
                >
                  {!filterText && <AddIcon sx={{ fontSize: 16, mr: 0.5 }} />}
                  {filterText ?? 'Add filter'}
                </Button>
              </span>
            </Tooltip>
          ) : (
            <Typography
              variant="body2"
              color="text.secondary"
              data-testid="irm-no-filters"
            >
              None
            </Typography>
          )}
        </Box>
        {isDialogOpen && (
          <IrmFilterDialog
            initialExpressions={rawExpressions}
            filterBuilders={filterBuilders}
            isBuildersLoading={isBuildersLoading}
            searchPlaceholder={searchPlaceholder}
            readOnly={!canEdit}
            onCancel={handleCloseDialog}
            onSave={handleSaveDialog}
          />
        )}
      </TableCell>
      <TableCell
        align="right"
        sx={{ verticalAlign: 'middle', whiteSpace: 'nowrap' }}
        data-testid="irm-affected-count"
      >
        <Box
          sx={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: 0.75,
          }}
        >
          {isSyncing && (
            <CircularProgress size={14} data-testid="irm-sync-spinner" />
          )}
          <Typography
            variant="body2"
            sx={{
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {affectedCount}
          </Typography>
        </Box>
      </TableCell>
    </TableRow>
  );
};

export const ActiveIrmTable = ({
  incidents,
  isLoading,
  isError,
  error,
  filterBuilders = EMPTY_FILTER_BUILDERS,
  isBuildersLoading = false,
  canEdit = false,
  isSyncingIncident,
  updateIncident,
  searchPlaceholder = 'Filter affected devices by board, model, pool...',
}: ActiveIrmTableProps) => {
  const [mutationError, setMutationError] = useState<string | null>(null);

  const handleUpdateExpressions = useCallback(
    async (incidentId: string, nextExpressions: readonly string[]) => {
      if (!updateIncident) return;
      setMutationError(null);
      try {
        await updateIncident({
          id: incidentId,
          expressionsAip160: nextExpressions,
        });
      } catch (err) {
        setMutationError(getErrorMessage(err, 'updating IRM incident filter'));
      }
    },
    [updateIncident],
  );

  return (
    <Box sx={{ width: '100%' }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Active IRM Bugs
      </Typography>

      {mutationError && (
        <Alert
          severity="error"
          sx={{ mb: 1 }}
          onClose={() => setMutationError(null)}
          data-testid="active-irm-mutation-error"
        >
          {mutationError}
        </Alert>
      )}

      {isLoading && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            p: 4,
          }}
          data-testid="active-irm-loading"
        >
          <CircularProgress size={28} />
        </Box>
      )}

      {isError && (
        <Alert severity="error" sx={{ my: 1 }} data-testid="active-irm-error">
          {getErrorMessage(error, 'loading active IRM incidents')}
        </Alert>
      )}

      {!isLoading && !isError && (
        <TableContainer
          sx={{
            maxHeight: 280,
            overflowY: 'auto',
            borderTop: borders.divider,
          }}
          data-testid="active-irm-table-container"
        >
          <Table
            size="small"
            stickyHeader
            aria-label="active IRM incidents table"
            sx={{
              tableLayout: 'fixed',
              '& .MuiTableCell-root': {
                px: 1,
              },
            }}
          >
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold', width: '120px' }}>
                  Bug ID
                </TableCell>
                <TableCell sx={{ fontWeight: 'bold', minWidth: '80px' }}>
                  Incident Name
                </TableCell>
                <TableCell sx={{ fontWeight: 'bold', width: '90px' }}>
                  Filters
                </TableCell>
                <TableCell
                  align="right"
                  sx={{
                    fontWeight: 'bold',
                    width: '75px',
                    fontSize: '0.75rem',
                    lineHeight: 1.15,
                  }}
                >
                  Affected Devices
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {incidents.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    align="center"
                    sx={{ color: 'text.secondary', py: 4 }}
                    data-testid="active-irm-empty"
                  >
                    No active IRM incidents
                  </TableCell>
                </TableRow>
              ) : (
                incidents.map((incident) => (
                  <ActiveIrmRow
                    key={incident.id}
                    incident={incident}
                    filterBuilders={filterBuilders}
                    isBuildersLoading={isBuildersLoading}
                    canEdit={canEdit}
                    isSyncing={isSyncingIncident?.(incident.id) ?? false}
                    searchPlaceholder={searchPlaceholder}
                    onUpdateExpressions={handleUpdateExpressions}
                  />
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
};
