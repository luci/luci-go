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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import CheckIcon from '@mui/icons-material/Check';
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import {
  Alert,
  Box,
  Button,
  IconButton,
  InputAdornment,
  Link,
  MenuItem,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_Row,
} from 'material-react-table';
import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { fleetTableHeaderSx } from '@/fleet/components/fc_data_table/fleet_column_header.styles';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import {
  CATALOG_PATH,
  generateCatalogDetailsURL,
} from '@/fleet/constants/paths';
import { useProductCatalogRevamp } from '@/fleet/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import {
  focusRingSx,
  iconButtonSx,
  surfaceCardClippedSx,
} from '@/fleet/theme/recipes';
import {
  controlHeights,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import {
  formatUnitCost,
  GCE_ONLY_ACCESSORS,
  getPlmStatusTone,
  HARDWARE_ONLY_ACCESSORS,
  resolveProductType,
} from './catalog_details_helpers';
import { CatalogDetailsHero } from './catalog_details_hero';
import { CatalogPhotoEditDialog } from './catalog_edit_prompt_dialog';
import {
  ALLOWED_PLM_STATUSES,
  CatalogEditableFieldKey,
  EDITABLE_SPEC_FIELD_BY_HEADER,
} from './catalog_edit_validation';
import { OrderForm } from './order_form';
import { COLUMNS } from './product_catalogue_columns';
import { UnifiedProductCatalogEntry } from './types';
import { UnsavedChangesToast } from './unsaved_changes_toast';
import { useCatalogEditState } from './use_catalog_edit_state';
import { useProductCatalogDetailsData } from './use_product_catalog_details_data';

interface DetailRow {
  readonly key: string;
  readonly rawValue: unknown;
  readonly entry?: UnifiedProductCatalogEntry;
  readonly draftValue?: string;
  readonly baselineValue?: string;
  readonly errorMsg?: string;
}

interface DetailValueCellProps {
  readonly row: MRT_Row<DetailRow>;
  readonly isRevampEnabled?: boolean;
  readonly isEditingSpecs?: boolean;
  readonly onStartEditingSpecs?: () => void;
  readonly onStopEditingSpecs?: () => void;
  readonly onSpecFieldChange?: (
    field: CatalogEditableFieldKey,
    value: string,
  ) => void;
}

const DetailValueCell = ({
  row,
  isRevampEnabled = false,
  isEditingSpecs = false,
  onStartEditingSpecs,
  onStopEditingSpecs,
  onSpecFieldChange,
}: DetailValueCellProps) => {
  const { key: fieldKey, rawValue, entry } = row.original;
  const editableFieldKey = EDITABLE_SPEC_FIELD_BY_HEADER[fieldKey];
  const isGce =
    resolveProductType(entry?.productType, entry?.productCatalogId) === 'gce';

  if (
    isEditingSpecs &&
    !isGce &&
    editableFieldKey &&
    row.original.draftValue !== undefined &&
    onSpecFieldChange
  ) {
    const currentVal = row.original.draftValue;
    const baselineVal = row.original.baselineValue ?? '';
    const isModified = currentVal.trim() !== baselineVal.trim();
    const errorMsg = row.original.errorMsg;
    const isPlmStatusField = editableFieldKey === 'fleetPlmStatus';
    const normalizedPlmVal = currentVal.trim().toUpperCase();
    const plmStatusOptions =
      isPlmStatusField &&
      normalizedPlmVal &&
      !ALLOWED_PLM_STATUSES.includes(
        normalizedPlmVal as (typeof ALLOWED_PLM_STATUSES)[number],
      )
        ? [...ALLOWED_PLM_STATUSES, normalizedPlmVal]
        : ALLOWED_PLM_STATUSES;

    const endAdornmentContent =
      errorMsg || isModified ? (
        <InputAdornment
          position="end"
          sx={{ gap: 0.5, mr: isPlmStatusField ? 2.5 : -0.5, maxWidth: '55%' }}
        >
          {errorMsg && (
            <Typography
              variant="caption"
              color="error"
              title={errorMsg}
              sx={{
                fontSize: fontSizes.caption,
                lineHeight: 1.1,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {errorMsg}
            </Typography>
          )}
          {isModified && (
            <Tooltip title={`Reset to ${baselineVal || 'empty'}`}>
              <IconButton
                size="small"
                aria-label={`Reset ${fieldKey}`}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.stopPropagation();
                  onSpecFieldChange(editableFieldKey, baselineVal);
                }}
                sx={{ p: 0.25, color: fleetColors.primary, flexShrink: 0 }}
              >
                <RestartAltIcon sx={{ fontSize: iconSizes.control }} />
              </IconButton>
            </Tooltip>
          )}
        </InputAdornment>
      ) : undefined;

    return (
      <Box
        sx={{
          width: '100%',
          height: controlHeights.button,
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <TextField
          select={isPlmStatusField}
          size="small"
          variant="outlined"
          fullWidth
          value={isPlmStatusField ? normalizedPlmVal : currentVal}
          error={Boolean(errorMsg)}
          title={errorMsg || undefined}
          onChange={(e) => onSpecFieldChange(editableFieldKey, e.target.value)}
          onKeyDown={(e) => {
            if (e.defaultPrevented) {
              return;
            }
            if (e.key === 'Escape') {
              e.preventDefault();
              e.stopPropagation();
              if (isModified) {
                onSpecFieldChange(editableFieldKey, baselineVal);
              } else {
                onStopEditingSpecs?.();
              }
            }
          }}
          sx={{
            flex: 1,
            '& .MuiOutlinedInput-root': {
              height: controlHeights.button,
              fontSize: fontSizes.label,
              backgroundColor: fleetColors.surface,
            },
          }}
          slotProps={{
            select: isPlmStatusField
              ? {
                  displayEmpty: true,
                  renderValue: (selected) => {
                    const statusStr = String(selected ?? '').trim();
                    return statusStr ? (
                      <StatusPill
                        tone={getPlmStatusTone(statusStr)}
                        size="compact"
                        dot={false}
                      >
                        {statusStr}
                      </StatusPill>
                    ) : (
                      <Box
                        component="span"
                        sx={{ color: fleetColors.textTertiary }}
                      >
                        Select status
                      </Box>
                    );
                  },
                  SelectDisplayProps: {
                    'aria-label': `Edit ${fieldKey}`,
                    ...({
                      'data-testid': `spec-select-${editableFieldKey}`,
                    } as Record<string, string>),
                  },
                }
              : undefined,
            input: endAdornmentContent
              ? { endAdornment: endAdornmentContent }
              : undefined,
            htmlInput: {
              'data-testid': `spec-input-${editableFieldKey}`,
              'aria-label': `Edit ${fieldKey}`,
            },
          }}
        >
          {isPlmStatusField && (
            <MenuItem value="">
              <em>None</em>
            </MenuItem>
          )}
          {isPlmStatusField &&
            plmStatusOptions.map((status) => (
              <MenuItem key={status} value={status}>
                <StatusPill
                  tone={getPlmStatusTone(status)}
                  size="compact"
                  dot={false}
                >
                  {status}
                </StatusPill>
              </MenuItem>
            ))}
        </TextField>
      </Box>
    );
  }

  const renderReadOnlyContent = () => {
    const col = COLUMNS.find((c) => c.header === fieldKey);
    if (!col) return <>{String(rawValue ?? '')}</>;

    const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;

    if (accessor === 'productCatalogId') {
      if (!isRevampEnabled) {
        return <>{String(rawValue ?? '')}</>;
      }
      return (
        <Typography
          variant="body2"
          sx={{
            fontFamily: fontFamilies.mono,
            fontWeight: 500,
          }}
        >
          {String(rawValue ?? '')}
        </Typography>
      );
    }

    if (isRevampEnabled && accessor === 'fleetPlmStatus') {
      const statusStr = String(rawValue ?? '').trim();
      if (!statusStr) return <>—</>;
      return (
        <StatusPill
          tone={getPlmStatusTone(statusStr)}
          size="compact"
          dot={false}
        >
          {statusStr}
        </StatusPill>
      );
    }

    if (isRevampEnabled && accessor === 'unitCost') {
      return <>{formatUnitCost(String(rawValue ?? ''))}</>;
    }

    if (accessor === 'r11n') {
      const items = (
        Array.isArray(rawValue)
          ? (rawValue as readonly string[])
          : [String(rawValue ?? '')]
      ).filter((v) => v.trim().length > 0);
      if (items.length === 0) return <>{isRevampEnabled ? '—' : ''}</>;
      return (
        <>
          {items.map((item, idx) => {
            const trimmed = item.trim();
            return (
              <span key={`${trimmed}-${idx}`}>
                {idx > 0 && ', '}
                <Link
                  href={`https://goto.google.com/ngp-npi/r11n/${encodeURIComponent(trimmed.toLowerCase())}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {trimmed}
                </Link>
              </span>
            );
          })}
        </>
      );
    }

    if (Array.isArray(rawValue)) {
      const joined = rawValue.join(', ').trim();
      return <>{joined || (isRevampEnabled ? '—' : '')}</>;
    }

    const strValue = String(rawValue ?? '').trim();
    if (!strValue && isRevampEnabled) {
      return <>—</>;
    }

    return <>{String(rawValue ?? '')}</>;
  };

  if (
    !isEditingSpecs &&
    isRevampEnabled &&
    !isGce &&
    editableFieldKey &&
    onStartEditingSpecs
  ) {
    const isModifiedInReadMode =
      row.original.draftValue !== undefined &&
      row.original.baselineValue !== undefined &&
      row.original.draftValue.trim() !== row.original.baselineValue.trim();

    return (
      <Box
        role={editableFieldKey === 'r11n' ? undefined : 'button'}
        aria-label={
          editableFieldKey === 'r11n' ? undefined : `Edit ${fieldKey}`
        }
        tabIndex={editableFieldKey === 'r11n' ? undefined : 0}
        data-testid={`spec-cell-${editableFieldKey}`}
        onClick={(e) => {
          if ((e.target as HTMLElement).closest('a')) return;
          onStartEditingSpecs();
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            if ((e.target as HTMLElement).closest('a')) return;
            e.preventDefault();
            onStartEditingSpecs();
          }
        }}
        title={`Click to edit ${fieldKey}`}
        sx={{
          ...focusRingSx,
          width: '100%',
          height: 32,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 1,
          px: 0.75,
          mx: -0.75,
          borderRadius: radii.sm,
          cursor: 'pointer',
          backgroundColor: 'transparent',
          '&:hover': {
            backgroundColor: fleetColors.surfaceTonal,
            '& .row-edit-affordance': {
              opacity: 1,
            },
          },
        }}
      >
        <Box sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
          {renderReadOnlyContent()}
        </Box>
        <EditOutlinedIcon
          className="row-edit-affordance"
          sx={{
            fontSize: 14,
            color: isModifiedInReadMode
              ? fleetColors.primary
              : fleetColors.textSecondary,
            opacity: isModifiedInReadMode ? 0.9 : 0,
            flexShrink: 0,
            transition: 'opacity 0.15s ease-in-out',
          }}
        />
      </Box>
    );
  }

  return renderReadOnlyContent();
};

const useNavigatedFromLink = () => {
  const { state } = useLocation();

  const [navigatedFromLink, setNavigatedFromLink] = useState<
    string | undefined
  >(undefined);

  // state from useLocation is lost on rerenders so we need to save it in a react state
  useEffect(() => {
    if (state) {
      setNavigatedFromLink(state.navigatedFromLink);
    }
  }, [state]);

  return navigatedFromLink;
};

export const ProductCatalogDetailsPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const navigatedFromLink = useNavigatedFromLink();
  const isRevampEnabled = useProductCatalogRevamp();

  const navigateToCatalogIfChanged = (newId: string) => {
    if (id !== newId) {
      navigate(generateCatalogDetailsURL(newId), {
        state: { navigatedFromLink },
      });
    }
  };

  const { error, isError, isLoading, entry } = useProductCatalogDetailsData(id);
  const editState = useCatalogEditState(id, entry);

  const resolvedProductType = resolveProductType(
    entry?.productType,
    entry?.productCatalogId,
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
                const editLabel = editState.isEditingSpecs
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
                        variant={
                          editState.isEditingSpecs ? 'contained' : 'outlined'
                        }
                        color="primary"
                        size="small"
                        aria-label={editLabel}
                        onClick={editState.handleToggleEditingSpecs}
                        data-testid="toggle-spec-editor-btn"
                        startIcon={
                          editState.isEditingSpecs ? (
                            <CheckIcon />
                          ) : (
                            <EditOutlinedIcon />
                          )
                        }
                      >
                        {editState.isEditingSpecs ? 'Done' : 'Edit'}
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
            isRevampEnabled={isRevampEnabled}
            isEditingSpecs={editState.isEditingSpecs}
            onStartEditingSpecs={editState.handleStartEditingSpecs}
            onStopEditingSpecs={editState.handleStopEditingSpecs}
            onSpecFieldChange={editState.handleSpecFieldChange}
          />
        ),
      },
    ],
    [
      editState.handleSpecFieldChange,
      editState.handleStartEditingSpecs,
      editState.handleStopEditingSpecs,
      editState.handleToggleEditingSpecs,
      editState.isEditingSpecs,
      isGceEntry,
      isRevampEnabled,
    ],
  );

  const data = useMemo<DetailRow[]>(() => {
    const activeEntry = editState.displayEntry ?? entry;
    if (!activeEntry) return [];
    const resolvedType = resolveProductType(
      activeEntry.productType,
      activeEntry.productCatalogId,
    );
    const isGce = resolvedType === 'gce';
    const filteredCols = isRevampEnabled
      ? COLUMNS.filter((col) => {
          const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;
          return isGce
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
          ? activeEntry.productType || resolvedType
          : activeEntry[accessor];
      return {
        key: header,
        rawValue: val,
        entry: activeEntry,
        draftValue: editableKey
          ? editState.activeSpecDraft[editableKey]
          : undefined,
        baselineValue: editableKey
          ? editState.baselineSpecDraft[editableKey]
          : undefined,
        errorMsg: editableKey ? editState.specErrors[editableKey] : undefined,
      };
    });
  }, [
    editState.activeSpecDraft,
    editState.baselineSpecDraft,
    editState.displayEntry,
    editState.specErrors,
    entry,
    isRevampEnabled,
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
    muiTableProps: isRevampEnabled
      ? {
          sx: {
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
          },
        }
      : undefined,
    muiTablePaperProps: isRevampEnabled
      ? {
          elevation: 0,
          sx: {
            border: 'none',
            borderRadius: 0,
            boxShadow: 'none',
          },
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

  if (isError) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">
          Something went wrong:{' '}
          {getErrorMessage(error, 'fetch product catalog entry')}
        </Alert>
      </Box>
    );
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', my: 4 }}>
        <CentralizedProgress data-testid="loading-spinner" />
      </Box>
    );
  }

  if (!isRevampEnabled) {
    return (
      <Box sx={{ p: 3 }} data-testid="legacy-catalog-details-view">
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            gap: 1,
            mb: 3,
          }}
        >
          <IconButton
            aria-label="go back"
            onClick={() => {
              if (navigatedFromLink) {
                navigate(navigatedFromLink);
              } else {
                navigate(CATALOG_PATH);
              }
            }}
            sx={{ mr: 2 }}
          >
            <ArrowBackIcon />
          </IconButton>
          <Typography variant="h4" component="h1" sx={{ whiteSpace: 'nowrap' }}>
            Product Catalog Entry:
          </Typography>
          <TextField
            key={id}
            variant="standard"
            defaultValue={id}
            slotProps={{ htmlInput: { sx: { fontSize: 24 } } }}
            fullWidth
            onBlur={(e) => {
              navigateToCatalogIfChanged(e.target.value);
            }}
            onKeyDown={(e) => {
              const target = e.target as HTMLInputElement;
              if (e.key === 'Enter') {
                navigateToCatalogIfChanged(target.value);
              }
            }}
          />
        </Box>

        {!entry ? (
          <AlertWithFeedback
            title="Entry not found!"
            bugErrorMessage={`Product Catalog Entry not found: ${id}`}
          >
            <p>
              The product catalog entry <code>{id}</code> you are looking for
              was not found.
            </p>
          </AlertWithFeedback>
        ) : (
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' },
              gap: 3,
              alignItems: 'start',
            }}
          >
            <Box sx={{ overflowX: 'auto', order: { xs: 2, lg: 1 } }}>
              <MaterialReactTable table={table} />
            </Box>
            <Box sx={{ order: { xs: 1, lg: 2 } }}>
              <OrderForm entry={entry} />
            </Box>
          </Box>
        )}
      </Box>
    );
  }

  const activeHeroEntry = editState.displayEntry ?? entry;

  return (
    <Box
      sx={{
        p: 3,
        pb:
          editState.dirtyCount > 0 || editState.validationErrorCount > 0
            ? 12
            : 3,
      }}
    >
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          gap: 1,
          mb: 2.5,
        }}
      >
        <IconButton
          aria-label="go back"
          onClick={() => {
            if (navigatedFromLink) {
              navigate(navigatedFromLink);
            } else {
              navigate(CATALOG_PATH);
            }
          }}
          sx={{ ...iconButtonSx, mr: 1 }}
        >
          <ArrowBackIcon
            sx={{ fontSize: iconSizes.inline, display: 'block' }}
          />
        </IconButton>
        <Typography
          variant="h5"
          component="h1"
          sx={{
            whiteSpace: 'nowrap',
            fontWeight: 700,
            color: fleetColors.textPrimary,
          }}
        >
          Product Catalog Entry:
        </Typography>
        <TextField
          key={id}
          variant="standard"
          defaultValue={id}
          slotProps={{
            htmlInput: {
              sx: {
                fontSize: fontSizes.title,
                fontWeight: 600,
                fontFamily: fontFamilies.mono,
              },
            },
          }}
          sx={{ maxWidth: 280 }}
          onBlur={(e) => {
            navigateToCatalogIfChanged(e.target.value);
          }}
          onKeyDown={(e) => {
            const target = e.target as HTMLInputElement;
            if (e.key === 'Enter') {
              navigateToCatalogIfChanged(target.value);
            }
          }}
        />
      </Box>

      {!activeHeroEntry ? (
        <AlertWithFeedback
          title="Entry not found!"
          bugErrorMessage={`Product Catalog Entry not found: ${id}`}
        >
          <p>
            The product catalog entry <code>{id}</code> you are looking for was
            not found.
          </p>
        </AlertWithFeedback>
      ) : (
        <>
          <CatalogDetailsHero
            key={activeHeroEntry.productCatalogId}
            entry={activeHeroEntry}
            stagedPhotos={editState.stagedPhotos}
            onOpenPhotoEditor={editState.handleOpenPhotoEditor}
            onPhotoDimensionsEvaluated={
              editState.handlePhotoDimensionsEvaluated
            }
          />
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', lg: '7fr 5fr' },
              gap: 3,
              alignItems: 'start',
            }}
          >
            {/* Left Column: Specifications Card */}
            <Box
              data-testid="catalog-specs-card"
              sx={{
                ...surfaceCardClippedSx,
                order: { xs: 2, lg: 1 },
              }}
            >
              <Box sx={{ overflowX: 'auto' }}>
                <MaterialReactTable table={table} />
              </Box>
            </Box>

            <Box sx={{ order: { xs: 1, lg: 2 } }}>
              <OrderForm entry={activeHeroEntry} />
            </Box>
          </Box>

          <CatalogPhotoEditDialog
            open={editState.isPhotoEditorOpen}
            onClose={editState.handleClosePhotoEditor}
            onCopyPrompt={editState.handleCopyPrompt}
            copiedPrompt={editState.copiedPrompt}
            prompt={editState.generatedPrompt}
            productCatalogId={activeHeroEntry.productCatalogId}
            productName={
              activeHeroEntry.productName || activeHeroEntry.descriptiveName
            }
            resolvedProductType={resolvedProductType}
            originalPhotos={editState.originalPhotos}
            stagedPhotos={editState.stagedPhotos}
            photoUrlDraft={editState.photoUrlDraft}
            photoInputError={editState.photoInputError}
            photoDimensionErrors={editState.photoDimensionErrors}
            onPhotoUrlDraftChange={editState.handlePhotoUrlDraftChange}
            onStagePhotoUrl={editState.handleStagePhotoUrl}
            onStageUploadedDataUrl={editState.handleStageUploadedDataUrl}
            onRemoveStagedPhoto={editState.handleRemoveStagedPhoto}
            onSetPrimaryPhoto={editState.handleSetPrimaryPhoto}
            onResetPhotos={editState.handleResetPhotos}
            onPhotoDimensionsEvaluated={
              editState.handlePhotoDimensionsEvaluated
            }
          />

          {!editState.isPhotoEditorOpen && (
            <UnsavedChangesToast
              dirtyCount={editState.dirtyCount}
              validationErrorCount={editState.validationErrorCount}
              errorSummaryText={editState.firstValidationErrorMessage}
              productCatalogId={activeHeroEntry.productCatalogId}
              prompt={editState.generatedPrompt}
              copiedPrompt={editState.copiedPrompt}
              onDiscard={editState.handleDiscardEdits}
              onCopyPrompt={editState.handleCopyPrompt}
            />
          )}
        </>
      )}
    </Box>
  );
};

export function Component() {
  const { id = '' } = useParams();
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-product-catalog-details">
      <FleetHelmet
        pageTitle={`${id ? `${id} | ` : ''}Product Catalog Details`}
      />
      <RecoverableErrorBoundary key="fleet-product-catalog-details-page">
        <LoggedInBoundary>
          <ProductCatalogDetailsPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
