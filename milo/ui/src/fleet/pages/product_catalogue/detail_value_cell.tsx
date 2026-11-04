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

import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import {
  Box,
  IconButton,
  InputAdornment,
  Link,
  MenuItem,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { MRT_Row } from 'material-react-table';

import { sortLabelValues } from '@/fleet/components/device_table/dimensions';
import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { DIMENSION_SEPARATOR } from '@/fleet/constants/dimension_separator';
import { focusRingSx } from '@/fleet/theme/recipes';
import {
  controlHeights,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
  statusTones,
} from '@/fleet/theme/tokens';

import { formatUnitCost, getPlmStatusTone } from './catalog_details_helpers';
import {
  ALLOWED_PLM_STATUSES,
  CatalogEditableFieldKey,
  EDITABLE_SPEC_FIELD_BY_HEADER,
} from './catalog_edit_validation';
import { COLUMNS } from './product_catalogue_columns';
import { UnifiedProductCatalogEntry } from './types';

export interface DetailRow {
  readonly key: string;
  readonly rawValue: unknown;
  readonly draftValue?: string;
  readonly baselineValue?: string;
  readonly errorMsg?: string;
}

export interface DetailValueCellProps {
  readonly row: MRT_Row<DetailRow>;
  readonly isGce?: boolean;
  readonly isRevampEnabled?: boolean;
  readonly isEditingSpecs?: boolean;
  readonly onStartEditingSpecs?: () => void;
  readonly onStopEditingSpecs?: () => void;
  readonly onSpecFieldChange?: (
    field: CatalogEditableFieldKey,
    value: string,
  ) => void;
}

const renderPlmStatusValue = (selected: unknown) => {
  const statusStr = String(selected ?? '').trim();
  return statusStr ? (
    <StatusPill tone={getPlmStatusTone(statusStr)} size="compact" dot={false}>
      {statusStr}
    </StatusPill>
  ) : (
    <Box component="span" sx={{ color: fleetColors.textTertiary }}>
      Select status
    </Box>
  );
};

export const DetailValueCell = ({
  row,
  isGce = false,
  isRevampEnabled = false,
  isEditingSpecs = false,
  onStartEditingSpecs,
  onStopEditingSpecs,
  onSpecFieldChange,
}: DetailValueCellProps) => {
  const { key: fieldKey, rawValue } = row.original;
  const editableFieldKey = EDITABLE_SPEC_FIELD_BY_HEADER[fieldKey];

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

    const resetButton = isModified ? (
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
    ) : null;

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
            if (e.defaultPrevented) return;
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
            '& .MuiInputBase-root': {
              height: controlHeights.button,
              fontSize: fontSizes.body,
              backgroundColor: fleetColors.surface,
              pr: isModified || errorMsg ? 0.5 : undefined,
            },
            '& .MuiOutlinedInput-input': { py: 0.5, px: 1 },
          }}
          slotProps={{
            select: isPlmStatusField
              ? {
                  displayEmpty: true,
                  renderValue: renderPlmStatusValue,
                  SelectDisplayProps: { 'aria-label': `Edit ${fieldKey}` },
                }
              : undefined,
            htmlInput: {
              'data-testid': `spec-input-${editableFieldKey}`,
              'aria-label': `Edit ${fieldKey}`,
            },
            input:
              isModified || errorMsg
                ? {
                    endAdornment: (
                      <InputAdornment
                        position="end"
                        sx={{
                          gap: 0.5,
                          maxHeight: 'none',
                          mr: isPlmStatusField ? 2.5 : 0,
                          maxWidth: '55%',
                        }}
                      >
                        {errorMsg && (
                          <Typography
                            variant="caption"
                            role="alert"
                            title={errorMsg}
                            sx={{
                              color: statusTones.error.fg,
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
                        {resetButton}
                      </InputAdornment>
                    ),
                  }
                : undefined,
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
                {renderPlmStatusValue(status)}
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

    if (accessor === 'productCatalogId' && isRevampEnabled) {
      return (
        <Typography
          variant="body2"
          sx={{ fontFamily: fontFamilies.mono, fontWeight: 500 }}
        >
          {String(rawValue ?? '')}
        </Typography>
      );
    }

    if (isRevampEnabled && accessor === 'fleetPlmStatus') {
      const statusStr = String(rawValue ?? '').trim();
      if (!statusStr) return <>—</>;
      return renderPlmStatusValue(statusStr);
    }

    if (isRevampEnabled && accessor === 'unitCost') {
      return <>{formatUnitCost(String(rawValue ?? ''))}</>;
    }

    if (accessor === 'r11n') {
      const items = sortLabelValues(
        Array.isArray(rawValue)
          ? (rawValue as readonly string[])
          : [String(rawValue ?? '')],
      )
        .map((v) => v.trim())
        .filter(Boolean);
      if (items.length === 0) return <>{isRevampEnabled ? '—' : ''}</>;
      return (
        <>
          {items.map((trimmed, idx) => (
            <span key={`${trimmed}-${idx}`}>
              {idx > 0 && DIMENSION_SEPARATOR}
              <Link
                href={`https://goto.google.com/ngp-npi/r11n/${encodeURIComponent(trimmed.toLowerCase())}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                {trimmed}
              </Link>
            </span>
          ))}
        </>
      );
    }

    if (Array.isArray(rawValue)) {
      const joined = rawValue.join(', ').trim();
      return <>{joined || (isRevampEnabled ? '—' : '')}</>;
    }

    const strValue = String(rawValue ?? '').trim();
    if (!strValue && isRevampEnabled) return <>—</>;
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
            '& .row-edit-affordance': { opacity: 1 },
          },
        }}
      >
        <Box sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
          {renderReadOnlyContent()}
        </Box>
        <EditOutlinedIcon
          className="row-edit-affordance"
          sx={{
            fontSize: iconSizes.control,
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
