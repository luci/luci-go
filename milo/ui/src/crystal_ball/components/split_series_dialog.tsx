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
  CallSplit as SplitIcon,
  CheckBox as CheckBoxIcon,
  CheckBoxOutlineBlank as CheckBoxOutlineBlankIcon,
  Close as CloseIcon,
} from '@mui/icons-material';
import {
  alpha,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Drawer,
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router';

import {
  useListMeasurementFilterColumns,
  useSuggestMeasurementFilterValues,
} from '@/crystal_ball/hooks/use_measurement_filter_api';
import {
  buildFilterString,
  generateColor,
  getColumnDisplayName,
  getFilterableColumns,
} from '@/crystal_ball/utils';
import {
  PerfChartSeries,
  PerfFilter,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

const icon = <CheckBoxOutlineBlankIcon fontSize="small" />;
const checkedIcon = <CheckBoxIcon fontSize="small" />;

/**
 * The maximum number of measurement filter suggestions to request when splitting series.
 */
export const MAX_SPLIT_SUGGEST_RESULTS = 20;

interface SplitSeriesDialogProps {
  open: boolean;
  onClose: () => void;
  series: PerfChartSeries | null;
  onSplit: (selectedValues: string[], column: string) => void;
  dataSpecId: string;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
}

export function SplitSeriesDialog({
  open,
  onClose,
  series,
  onSplit,
  dataSpecId,
  globalFilters,
  widgetFilters,
}: SplitSeriesDialogProps) {
  const { dashboardId } = useParams<{ dashboardId: string }>();
  const parent = dashboardId
    ? `dashboardStates/${dashboardId}/dataSpecs/${dataSpecId}`
    : '';

  const [selectedColumn, setSelectedColumn] = useState<string | null>(null);
  const [selectedValues, setSelectedValues] = useState<string[]>([]);
  const [inputValue, setInputValue] = useState('');

  useEffect(() => {
    if (open) {
      setSelectedColumn(null);
      setSelectedValues([]);
      setInputValue('');
    }
  }, [open]);

  const { data: columnsData, isLoading: isLoadingColumns } =
    useListMeasurementFilterColumns(
      { parent, pageSize: 1000, pageToken: '' },
      { enabled: !!parent && open },
    );

  const columns = useMemo(() => {
    return getFilterableColumns(columnsData?.measurementFilterColumns ?? []);
  }, [columnsData]);

  const filterString = useMemo(() => {
    return buildFilterString([
      ...(globalFilters ?? []),
      ...(widgetFilters ?? []),
      ...(series?.filters ?? []),
    ]);
  }, [globalFilters, widgetFilters, series]);

  const { data: suggestionData, isLoading: isLoadingSuggestions } =
    useSuggestMeasurementFilterValues(
      {
        parent,
        column: selectedColumn ?? '',
        query: inputValue,
        maxResultCount: MAX_SPLIT_SUGGEST_RESULTS,
        filter: filterString,
        skipCache: true,
      },
      { enabled: !!parent && !!selectedColumn && open },
    );

  const options = suggestionData?.suggestions?.map((s) => s.value) ?? [];

  const handleSplit = () => {
    if (selectedColumn && selectedValues.length > 0) {
      onSplit(selectedValues, selectedColumn);
      onClose();
    }
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      disableScrollLock
      sx={{
        zIndex: (theme) => theme.zIndex.modal + 10,
      }}
      PaperProps={{
        sx: {
          width: { xs: '100%', sm: 480 },
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          boxShadow: (theme) => theme.shadows[24],
        },
      }}
    >
      {/* Header */}
      <Box
        sx={{
          p: 2.5,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 40,
              height: 40,
              borderRadius: '12px',
              bgcolor: (theme) => alpha(theme.palette.primary.main, 0.1),
              color: 'primary.main',
            }}
          >
            <SplitIcon />
          </Box>
          <Box>
            <Typography
              variant="subtitle1"
              sx={{ fontWeight: 700, lineHeight: 1.2 }}
            >
              Split Series
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.25 }}
            >
              Break down a series by dimension values
            </Typography>
          </Box>
        </Box>
        <IconButton
          onClick={onClose}
          size="small"
          edge="end"
          aria-label="close"
        >
          <CloseIcon />
        </IconButton>
      </Box>

      {/* Scrollable Content */}
      <Box
        sx={{
          flex: 1,
          overflowY: 'auto',
          p: 3,
          display: 'flex',
          flexDirection: 'column',
          gap: 3,
        }}
      >
        {/* Instructions Alert */}
        <Box
          sx={{
            p: 2,
            borderRadius: 2,
            bgcolor: (theme) => alpha(theme.palette.info.main, 0.06),
            border: '1px solid',
            borderColor: (theme) => alpha(theme.palette.info.main, 0.15),
            display: 'flex',
            flexDirection: 'column',
            gap: 0.5,
          }}
        >
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              color: 'info.main',
              display: 'flex',
              alignItems: 'center',
              gap: 0.5,
              textTransform: 'uppercase',
              letterSpacing: 0.5,
            }}
          >
            💡 How Splitting Works
          </Typography>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ fontSize: '0.8rem', lineHeight: 1.4 }}
          >
            Select a dimension to split this series by. The values you pick will
            create separate, filtered child series.
          </Typography>
        </Box>

        {/* Step 1: Select Dimension */}
        <Box>
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              color: 'text.secondary',
              textTransform: 'uppercase',
              letterSpacing: 1,
              mb: 1,
              display: 'block',
            }}
          >
            Step 1: Choose Dimension
          </Typography>
          <Autocomplete
            options={columns}
            getOptionLabel={getColumnDisplayName}
            value={columns.find((c) => c.column === selectedColumn) ?? null}
            onChange={(_event, newValue) => {
              setSelectedColumn(newValue?.column ?? null);
              setSelectedValues([]);
            }}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Select Dimension to Split On"
                placeholder="Search dimensions..."
                size="small"
              />
            )}
            loading={isLoadingColumns}
          />
        </Box>

        {/* Step 2: Select Values */}
        {selectedColumn && (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Box>
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  mb: 1,
                }}
              >
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 700,
                    color: 'text.secondary',
                    textTransform: 'uppercase',
                    letterSpacing: 1,
                  }}
                >
                  Step 2: Select Values
                </Typography>
                {options.length > 0 && (
                  <Button
                    size="small"
                    onClick={() => {
                      if (selectedValues.length === options.length) {
                        setSelectedValues([]);
                      } else {
                        setSelectedValues([...options]);
                      }
                    }}
                    sx={{ textTransform: 'none', py: 0, minWidth: 0 }}
                  >
                    {selectedValues.length === options.length
                      ? 'Deselect All'
                      : 'Select All'}
                  </Button>
                )}
              </Box>
              <Autocomplete
                multiple
                options={options}
                disableCloseOnSelect
                getOptionLabel={(option) => option}
                value={selectedValues}
                onChange={(_event, newValue) => {
                  setSelectedValues(newValue);
                }}
                inputValue={inputValue}
                onInputChange={(_event, newInputValue) => {
                  setInputValue(newInputValue);
                }}
                renderOption={(props, option, { selected }) => {
                  // eslint-disable-next-line react/prop-types
                  const { key, ...otherProps } = props;
                  return (
                    <li key={key} {...otherProps}>
                      <Checkbox
                        icon={icon}
                        checkedIcon={checkedIcon}
                        style={{ marginRight: 8 }}
                        checked={selected}
                      />
                      {option}
                    </li>
                  );
                }}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="Select Values"
                    placeholder={
                      selectedValues.length > 0 ? '' : 'Search values...'
                    }
                    size="small"
                  />
                )}
                loading={isLoadingSuggestions}
              />
            </Box>

            {/* Step 3: Live Preview */}
            {selectedValues.length > 0 && (
              <Box
                sx={{
                  mt: 1,
                  p: 2,
                  borderRadius: 2,
                  border: '1px dashed',
                  borderColor: 'divider',
                  bgcolor: (theme) => alpha(theme.palette.action.hover, 0.3),
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 1,
                }}
              >
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 700,
                    color: 'text.secondary',
                    textTransform: 'uppercase',
                  }}
                >
                  Preview of new series to be created ({selectedValues.length})
                </Typography>
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 0.75,
                    maxHeight: 180,
                    overflowY: 'auto',
                  }}
                >
                  {selectedValues.map((val, i) => (
                    <Box
                      key={val}
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        p: 0.75,
                        borderRadius: 1,
                        bgcolor: 'background.paper',
                        border: '1px solid',
                        borderColor: 'divider',
                      }}
                    >
                      <Box
                        sx={{
                          width: 8,
                          height: 8,
                          borderRadius: '50%',
                          bgcolor: generateColor(
                            series ? series.id.length + i : i,
                          ),
                        }}
                      />
                      <Typography
                        variant="body2"
                        noWrap
                        sx={{ fontSize: '0.8rem', fontWeight: 500 }}
                      >
                        {series?.displayName || 'Series'} - {val}
                      </Typography>
                    </Box>
                  ))}
                </Box>
              </Box>
            )}
          </Box>
        )}
      </Box>

      {/* Sticky Actions Footer */}
      <Box
        sx={{
          p: 2.5,
          borderTop: '1px solid',
          borderColor: 'divider',
          bgcolor: 'background.paper',
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 1.5,
        }}
      >
        <Button
          onClick={onClose}
          variant="outlined"
          color="inherit"
          sx={{ px: 3 }}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSplit}
          variant="contained"
          disabled={!selectedColumn || selectedValues.length === 0}
          sx={{
            px: 4,
            boxShadow: (theme) =>
              `0 4px 12px ${alpha(theme.palette.primary.main, 0.2)}`,
            '&:hover': {
              boxShadow: (theme) =>
                `0 6px 16px ${alpha(theme.palette.primary.main, 0.3)}`,
            },
          }}
        >
          Split Series
        </Button>
      </Box>
    </Drawer>
  );
}
