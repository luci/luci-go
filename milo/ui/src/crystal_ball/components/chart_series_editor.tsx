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
  Add as AddIcon,
  BarChart as BarChartIcon,
  ChevronRight as ChevronRightIcon,
  Delete as DeleteIcon,
  Edit as EditIcon,
  FilterAlt as FunnelIcon,
  Palette as PaletteIcon,
  Visibility as VisibilityIcon,
  VisibilityOff as VisibilityOffIcon,
} from '@mui/icons-material';
import {
  alpha,
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Box,
  Button,
  Chip,
  Divider,
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';
import { useParams } from 'react-router';
import { useDebounce } from 'react-use';

import { FilterEditor } from '@/crystal_ball/components';
import {
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  Column,
  COMMON_MESSAGES,
  GLOBAL_TIME_RANGE_COLUMN,
  MAX_SUGGEST_RESULTS,
  OPERATOR_DISPLAY_NAMES,
} from '@/crystal_ball/constants';
import {
  EditorUiKeyPrefix,
  useEditorUiState,
  UseEditorUiStateOptions,
  useSuggestMeasurementFilterValues,
} from '@/crystal_ball/hooks';
import {
  BackgroundAlpha,
  COMPACT_ICON_SX,
  COMPACT_TEXTFIELD_SX,
} from '@/crystal_ball/styles';
import {
  buildFilterString,
  isStringArray,
  generateColor,
} from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartSeries,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface ChartSeriesEditorProps {
  series: readonly PerfChartSeries[];
  onUpdateSeries: (updatedSeries: PerfChartSeries[]) => void;
  hiddenSeriesNames?: Set<string>;
  onToggleVisibility?: (name: string) => void;
  dataSpecId: string;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns?: boolean;
}

export function ChartSeriesEditor({
  series,
  onUpdateSeries,
  hiddenSeriesNames,
  onToggleVisibility,
  dataSpecId,
  globalFilters,
  widgetFilters,
  filterColumns,
  isLoadingFilterColumns,
}: ChartSeriesEditorProps) {
  const handleAddSeries = () => {
    const id = crypto.randomUUID();
    const newSeries: PerfChartSeries = PerfChartSeries.fromPartial({
      id,
      displayName: `series-${id}`,
      metricField: '',
      dataSpecId: dataSpecId,
      color: generateColor(series.length),
      filters: [],
    });
    onUpdateSeries([...series, newSeries]);
  };

  const handleUpdateSeriesItem = useCallback(
    (index: number, updatedItem: PerfChartSeries) => {
      const updatedSeries = [...series];
      updatedSeries[index] = updatedItem;
      onUpdateSeries(updatedSeries);
    },
    [series, onUpdateSeries],
  );

  const handleRemoveSeries = useCallback(
    (index: number) => {
      const updatedSeries = [...series];
      updatedSeries.splice(index, 1);
      onUpdateSeries(updatedSeries);
    },
    [series, onUpdateSeries],
  );

  const metricFilterColumns = useMemo(
    () =>
      filterColumns.filter(
        (c) =>
          c.applicableScopes?.includes(
            MeasurementFilterColumn_FilterScope.METRIC,
          ) ||
          (isStringArray(c.applicableScopes) &&
            c.applicableScopes.includes('METRIC')),
      ),
    [filterColumns],
  );

  return (
    <Box
      sx={{
        mt: 1,
        borderTop: '1px solid',
        borderColor: 'divider',
        mx: -2,
        mb: -2,
      }}
    >
      {series.map((s, index) => {
        const key = s.id ?? index;
        return (
          <ChartSeriesItem
            key={key}
            uiStateOptions={{ key }}
            series={s}
            onUpdate={(updatedItem) =>
              handleUpdateSeriesItem(index, updatedItem)
            }
            onRemove={() => handleRemoveSeries(index)}
            dataSpecId={dataSpecId}
            globalFilters={globalFilters}
            widgetFilters={widgetFilters}
            metricFilterColumns={metricFilterColumns}
            isLoadingColumns={isLoadingFilterColumns}
            isVisible={!hiddenSeriesNames?.has(s.displayName)}
            onToggleVisibility={() => onToggleVisibility?.(s.displayName)}
          />
        );
      })}
      <Button
        onClick={handleAddSeries}
        variant="text"
        fullWidth
        startIcon={<AddIcon />}
        sx={{
          justifyContent: 'flex-start',
          px: 2,
          py: 0,
          minHeight: (theme) => theme.spacing(5),
          color: 'primary.main',
          textTransform: 'none',
          typography: 'body2',
          fontWeight: (theme) => theme.typography.fontWeightBold,
        }}
      >
        {COMMON_MESSAGES.ADD_FILTER_METRIC_SERIES}
      </Button>
    </Box>
  );
}

/**
 * Props for the ChartSeriesItem component.
 */
export interface ChartSeriesItemProps {
  series: PerfChartSeries;
  onUpdate: (updatedSeries: PerfChartSeries) => void;
  onRemove: () => void;
  dataSpecId: string;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
  metricFilterColumns: readonly MeasurementFilterColumn[];
  isLoadingColumns?: boolean;
  hideColorPicker?: boolean;
  isVisible?: boolean;
  onToggleVisibility?: () => void;
  hideVisibility?: boolean;
  hideDelete?: boolean;
  titlePlaceholder?: string;
  uiStateOptions?: UseEditorUiStateOptions;
}

export function ChartSeriesItem({
  series,
  onUpdate,
  onRemove,
  dataSpecId,
  globalFilters,
  widgetFilters,
  metricFilterColumns,
  isLoadingColumns,
  hideColorPicker,
  isVisible = true,
  onToggleVisibility,
  hideVisibility = false,
  hideDelete = false,
  titlePlaceholder,
  uiStateOptions,
}: ChartSeriesItemProps) {
  const [expanded, setExpanded] = useEditorUiState({
    initialValue: false,
    prefix: EditorUiKeyPrefix.CHART_SERIES,
    ...uiStateOptions,
  });
  const [displayName, setDisplayName] = useState(series.displayName);
  const [color, setColor] = useState(series.color);
  const [inputValue, setInputValue] = useState(series.metricField);
  const [debouncedQuery, setDebouncedQuery] = useState(inputValue);
  const [isFocused, setIsFocused] = useState(false);

  const isShowingPlaceholder =
    (series.displayName ?? '') === '' &&
    (series.metricField ?? '') === '' &&
    (titlePlaceholder ?? '') !== '';

  useDebounce(
    () => {
      setDebouncedQuery(inputValue);
    },
    AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
    [inputValue],
  );

  const { dashboardId } = useParams<{ dashboardId: string }>();
  const parent = dashboardId
    ? `dashboardStates/${dashboardId}/dataSpecs/${dataSpecId}`
    : '';

  const filterString = useMemo(() => {
    return buildFilterString(
      [Column.ATP_TEST_NAME, GLOBAL_TIME_RANGE_COLUMN],
      globalFilters,
      widgetFilters,
    );
  }, [globalFilters, widgetFilters]);

  const hasAtpTestFilter = useMemo(() => {
    return new RegExp(`\\b${Column.ATP_TEST_NAME}\\b`).test(filterString);
  }, [filterString]);

  const { data: suggestionData, isLoading: isLoadingSuggestions } =
    useSuggestMeasurementFilterValues(
      {
        parent,
        column: 'metric_key',
        query: debouncedQuery,
        maxResultCount: MAX_SUGGEST_RESULTS,
        filter: filterString,
      },
      {
        enabled:
          !!parent &&
          debouncedQuery.length > 0 &&
          isFocused &&
          hasAtpTestFilter,
        retry: false,
      },
    );

  const options = suggestionData?.values || [];

  const handleBlurDisplayName = () => {
    if (displayName !== series.displayName) {
      onUpdate({ ...series, displayName });
    }
  };

  const handleBlurColor = () => {
    if (color !== series.color) {
      onUpdate({ ...series, color });
    }
  };

  const handleMetricFieldChange = (newValue: string) => {
    setInputValue(newValue);
    const isDefaultOrEmpty =
      (series.displayName ?? '') === '' ||
      series.displayName.startsWith('series-');
    const newDisplayName = isDefaultOrEmpty
      ? newValue !== ''
        ? newValue
        : series.displayName
      : series.displayName;
    setDisplayName(newDisplayName);
    onUpdate({
      ...series,
      metricField: newValue,
      displayName: newDisplayName,
    });
  };

  const handleUpdateFilters = (updatedFilters: PerfFilter[]) => {
    onUpdate({ ...series, filters: updatedFilters });
  };

  return (
    <Accordion
      expanded={expanded}
      onChange={() => setExpanded(!expanded)}
      disableGutters
      sx={{
        boxShadow: 'none',
        borderBottom: '1px solid',
        borderColor: 'divider',
        '&:before': { display: 'none' },
        '&.Mui-expanded': { m: 0 },
        '& .MuiAccordionSummary-root': { px: 2 },
        '& .MuiAccordionSummary-root:hover': { bgcolor: 'action.hover' },
        '& .MuiAccordionDetails-root': {
          px: 2,
          pb: 1,
          bgcolor: (theme) =>
            alpha(theme.palette.action.hover, BackgroundAlpha.HIGH),
        },
      }}
    >
      <AccordionSummary
        component="div"
        expandIcon={null}
        aria-controls={`series-${series.displayName}-content`}
        id={`series-${series.displayName}-header`}
        sx={{
          py: 0,
          minHeight: (theme) => theme.spacing(5),
          '&.Mui-expanded': {
            minHeight: (theme) => theme.spacing(5),
          },
          '& .MuiAccordionSummary-content': {
            alignItems: 'center',
            gap: 0.5,
            width: '100%',
            margin: '4px 0',
          },
          '&.Mui-expanded .MuiAccordionSummary-content': {
            margin: '4px 0',
          },
        }}
      >
        {!isShowingPlaceholder && (
          <ChevronRightIcon
            fontSize="small"
            sx={{
              transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
              transition: 'transform 0.2s',
              cursor: 'pointer',
            }}
          />
        )}
        {isShowingPlaceholder && <AddIcon fontSize="small" color="primary" />}
        {!hideColorPicker && (
          <Box
            data-testid="series-color-circle"
            sx={{
              width: 16,
              height: 16,
              bgcolor: series.color ?? color,
              borderRadius: '50%',
              border: '1px solid',
              borderColor: 'divider',
            }}
          />
        )}
        <Typography
          variant="body2"
          sx={{
            fontWeight: (theme) => theme.typography.fontWeightBold,
            color:
              !series.displayName && !series.metricField && titlePlaceholder
                ? 'primary.main'
                : 'inherit',
          }}
        >
          {series.displayName ||
            series.metricField ||
            titlePlaceholder ||
            'Untitled Series'}
        </Typography>
        {!expanded && series.filters?.length > 0 && (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, ml: 1 }}>
            {series.filters.map((filter) => {
              const op =
                filter.textInput?.defaultValue?.filterOperator !== undefined
                  ? perfFilterDefault_FilterOperatorFromJSON(
                      filter.textInput.defaultValue.filterOperator,
                    )
                  : PerfFilterDefault_FilterOperator.EQUAL;
              const val = filter.textInput?.defaultValue?.values?.[0] ?? '';
              const label = `${filter.column} ${OPERATOR_DISPLAY_NAMES[op] ?? PerfFilterDefault_FilterOperator[op]} "${val}"`;
              return (
                <Chip
                  key={filter.id}
                  label={label}
                  size="small"
                  variant="outlined"
                />
              );
            })}
          </Box>
        )}
        {!hideVisibility && (
          <IconButton
            onClick={(e) => {
              e.stopPropagation();
              onToggleVisibility?.();
            }}
            aria-label="Toggle series visibility"
            size="small"
            sx={{ ml: 'auto', p: 0.25 }}
          >
            {isVisible ? (
              <VisibilityIcon fontSize="small" />
            ) : (
              <VisibilityOffIcon fontSize="small" />
            )}
          </IconButton>
        )}
        {!hideVisibility && !hideDelete && (
          <Divider
            orientation="vertical"
            flexItem
            sx={{ mx: 0.25, height: 16 }}
          />
        )}
        {!hideDelete && (
          <IconButton
            onClick={(e) => {
              e.stopPropagation();
              onRemove();
            }}
            aria-label="Remove series"
            color="error"
            size="small"
            sx={{ p: 0.25 }}
          >
            <DeleteIcon fontSize="small" />
          </IconButton>
        )}
      </AccordionSummary>
      <AccordionDetails>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: hideColorPicker
                ? '1fr'
                : (theme) => `${theme.spacing(8)} 1fr`,
              gap: 1.5,
              alignItems: 'flex-start',
            }}
          >
            {!hideColorPicker && (
              <Box>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.5,
                    mb: 0.5,
                  }}
                >
                  <PaletteIcon sx={COMPACT_ICON_SX} />
                  <Typography
                    variant="caption"
                    sx={{
                      color: 'text.secondary',
                      fontWeight: (theme) => theme.typography.fontWeightBold,
                      textTransform: 'uppercase',
                    }}
                  >
                    {COMMON_MESSAGES.COLOR}
                  </Typography>
                </Box>
                <TextField
                  value={color}
                  onChange={(e) => setColor(e.target.value)}
                  onBlur={handleBlurColor}
                  size="small"
                  sx={{
                    width: (theme) => theme.spacing(6),
                    height: (theme) => theme.spacing(4),
                    '& .MuiOutlinedInput-notchedOutline': { border: 'none' },
                    '& .MuiInputBase-root': {
                      height: (theme) => theme.spacing(4),
                      width: (theme) => theme.spacing(6),
                      p: 0,
                    },
                    '& .MuiInputBase-input': {
                      p: 0,
                      width: '100%',
                      height: '100%',
                      cursor: 'pointer',
                      border: 'none',
                    },
                  }}
                  inputProps={{
                    'aria-label': 'Color',
                    type: 'color',
                  }}
                />
              </Box>
            )}
            <Box>
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.5,
                  mb: 0.5,
                }}
              >
                <EditIcon sx={COMPACT_ICON_SX} />
                <Typography
                  variant="caption"
                  sx={{
                    color: 'text.secondary',
                    fontWeight: (theme) => theme.typography.fontWeightBold,
                    textTransform: 'uppercase',
                  }}
                >
                  {COMMON_MESSAGES.DISPLAY_NAME}
                </Typography>
              </Box>
              <TextField
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                onBlur={handleBlurDisplayName}
                size="small"
                fullWidth
                inputProps={{
                  'aria-label': 'Display Name',
                }}
                sx={COMPACT_TEXTFIELD_SX}
              />
            </Box>
          </Box>

          <Box>
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.5,
                mb: 0.5,
              }}
            >
              <BarChartIcon sx={COMPACT_ICON_SX} />
              <Typography
                variant="caption"
                sx={{
                  color: 'text.secondary',
                  fontWeight: (theme) => theme.typography.fontWeightBold,
                  textTransform: 'uppercase',
                }}
              >
                {COMMON_MESSAGES.METRIC_FIELD}
              </Typography>
            </Box>
            <Autocomplete
              freeSolo
              size="small"
              options={options}
              filterOptions={(x) => x}
              value={series.metricField ?? null}
              inputValue={inputValue}
              onInputChange={(_event, newInputValue) => {
                setInputValue(newInputValue);
              }}
              onChange={(_event, newValue, reason) => {
                if (reason === 'selectOption' || reason === 'createOption') {
                  if (typeof newValue === 'string') {
                    handleMetricFieldChange(newValue);
                  }
                }
              }}
              onFocus={() => setIsFocused(true)}
              onBlur={() => {
                setIsFocused(false);
                if (inputValue !== series.metricField) {
                  handleMetricFieldChange(inputValue);
                }
              }}
              loading={isLoadingSuggestions && isFocused}
              renderInput={(params) => (
                <TextField
                  {...params}
                  placeholder="e.g., MemAvailable_CacheProcDirty_bytes"
                  inputProps={{
                    ...params.inputProps,
                    'aria-label': 'Metric Field',
                  }}
                  sx={COMPACT_TEXTFIELD_SX}
                />
              )}
            />
          </Box>

          <FilterEditor
            title="SERIES FILTERS"
            titleIcon={<FunnelIcon sx={COMPACT_ICON_SX} />}
            filters={[...(series.filters ?? [])]}
            onUpdateFilters={handleUpdateFilters}
            dataSpecId={dataSpecId}
            availableColumns={metricFilterColumns}
            isLoadingColumns={isLoadingColumns}
            disableAccordion={true}
            globalFilters={globalFilters}
          />
        </Box>
      </AccordionDetails>
    </Accordion>
  );
}
