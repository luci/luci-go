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
  CheckBox as CheckBoxIcon,
  CheckBoxOutlineBlank as CheckBoxOutlineBlankIcon,
} from '@mui/icons-material';
import {
  Autocomplete,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { useState, useMemo, useEffect } from 'react';
import { useParams } from 'react-router';

import { MAX_SUGGEST_RESULTS } from '@/crystal_ball/constants';
import {
  useListMeasurementFilterColumns,
  useSuggestMeasurementFilterValues,
} from '@/crystal_ball/hooks/use_measurement_filter_api';
import { buildFilterString } from '@/crystal_ball/utils';
import { PerfChartSeries } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

const icon = <CheckBoxOutlineBlankIcon fontSize="small" />;
const checkedIcon = <CheckBoxIcon fontSize="small" />;

interface SplitSeriesDialogProps {
  open: boolean;
  onClose: () => void;
  series: PerfChartSeries | null;
  onSplit: (selectedValues: string[], column: string) => void;
  dataSpecId: string;
}

export function SplitSeriesDialog({
  open,
  onClose,
  series,
  onSplit,
  dataSpecId,
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

  const columns = useMemo(
    () => columnsData?.measurementFilterColumns ?? [],
    [columnsData],
  );

  const filterString = useMemo(() => {
    return series ? buildFilterString(series.filters ?? []) : '';
  }, [series]);

  const { data: suggestionData, isLoading: isLoadingSuggestions } =
    useSuggestMeasurementFilterValues(
      {
        parent,
        column: selectedColumn ?? '',
        query: inputValue,
        maxResultCount: MAX_SUGGEST_RESULTS,
        filter: filterString,
      },
      { enabled: !!parent && !!selectedColumn && open },
    );

  const options = suggestionData?.values ?? [];

  const handleSplit = () => {
    if (selectedColumn && selectedValues.length > 0) {
      onSplit(selectedValues, selectedColumn);
      onClose();
    }
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Split Series</DialogTitle>
      <DialogContent>
        <Autocomplete
          options={columns.map((c) => c.column)}
          getOptionLabel={(option) => option}
          value={selectedColumn}
          onChange={(_event, newValue) => {
            setSelectedColumn(newValue);
            setSelectedValues([]);
          }}
          renderInput={(params) => (
            <TextField
              {...params}
              label="Select Dimension to Split On"
              margin="normal"
            />
          )}
          loading={isLoadingColumns}
        />
        {selectedColumn && (
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
            renderOption={(props, option, { selected }) => (
              <li {...props}>
                <Checkbox
                  icon={icon}
                  checkedIcon={checkedIcon}
                  style={{ marginRight: 8 }}
                  checked={selected}
                />
                {option}
              </li>
            )}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Select Values"
                placeholder="Search values..."
                margin="normal"
              />
            )}
            loading={isLoadingSuggestions}
          />
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          onClick={handleSplit}
          variant="contained"
          disabled={!selectedColumn || selectedValues.length === 0}
        >
          Split
        </Button>
      </DialogActions>
    </Dialog>
  );
}
