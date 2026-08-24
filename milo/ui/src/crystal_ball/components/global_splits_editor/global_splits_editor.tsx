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
  CallSplit as SplitIcon,
  Close as CloseIcon,
} from '@mui/icons-material';
import {
  Autocomplete,
  Box,
  Button,
  Chip,
  Drawer,
  IconButton,
  alpha,
  TextField,
  Typography,
  useTheme,
} from '@mui/material';
import { useState } from 'react';

import { DIMENSION_PREFIX, Z_INDEX } from '@/crystal_ball/constants';
import {
  getColumnDisplayName,
  getSplittableColumns,
} from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  PerfSeriesSplit,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { DimBadge } from '../raw_sample_list';

interface GlobalSplitsEditorProps {
  splits: readonly PerfSeriesSplit[];
  onUpdateSplits: (updatedSplits: PerfSeriesSplit[]) => void;
  availableColumns: readonly MeasurementFilterColumn[];
}

export function GlobalSplitsEditor({
  splits,
  onUpdateSplits,
  availableColumns,
}: GlobalSplitsEditorProps) {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const [selectedColumn, setSelectedColumn] =
    useState<MeasurementFilterColumn | null>(null);
  const [limitCount, setLimitCount] = useState<number>(10);

  const handleAdd = () => {
    if (!selectedColumn) return;

    const newSplit = PerfSeriesSplit.fromPartial({
      invocationDimension: selectedColumn.isMetricKey
        ? undefined
        : selectedColumn.column,
      metricDimension: selectedColumn.isMetricKey
        ? selectedColumn.column
        : undefined,
      limitCount,
    });

    onUpdateSplits([...splits, newSplit]);
    setOpen(false);
    setSelectedColumn(null);
    setLimitCount(10);
  };

  const handleRemove = (index: number) => {
    const updated = [...splits];
    updated.splice(index, 1);
    onUpdateSplits(updated);
  };

  const getSplitDisplayName = (split: PerfSeriesSplit) => {
    const colName = split.invocationDimension || split.metricDimension || '';
    const colList = availableColumns.find((c) => c.column === colName);
    const displayName = colList ? getColumnDisplayName(colList) : colName;
    return `${displayName} (top ${split.limitCount || 500})`;
  };

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        p: 1.5,
        gap: 1,
        flexWrap: 'wrap',
      }}
    >
      <Typography
        variant="body2"
        sx={{ fontWeight: 'bold', color: 'text.secondary', mr: 1 }}
      >
        Global Splits:
      </Typography>

      {splits.map((split, i) => (
        <Chip
          key={i}
          label={getSplitDisplayName(split)}
          onDelete={() => handleRemove(i)}
          size="small"
        />
      ))}

      <Button
        startIcon={<AddIcon />}
        onClick={() => setOpen(true)}
        variant="text"
        size="small"
        sx={{ textTransform: 'none', ml: 1 }}
      >
        Add Split
      </Button>

      <Drawer
        anchor="right"
        open={open}
        onClose={() => setOpen(false)}
        disableScrollLock
        sx={{
          zIndex: Z_INDEX.SIDE_DRAWER(theme),
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
        ModalProps={{
          role: 'dialog',
          'aria-labelledby': 'global-split-series-title',
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
                id="global-split-series-title"
                variant="subtitle1"
                sx={{ fontWeight: 700, lineHeight: 1.2 }}
              >
                Add Global Split
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: 'block', mt: 0.25 }}
              >
                Break down all charts by dimension
              </Typography>
            </Box>
          </Box>
          <IconButton
            onClick={() => setOpen(false)}
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
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Typography
              variant="caption"
              sx={{
                fontWeight: 700,
                color: 'text.secondary',
                textTransform: 'uppercase',
                letterSpacing: 1,
              }}
            >
              Step 1: Select Dimension
            </Typography>
            <Autocomplete
              options={getSplittableColumns(availableColumns)}
              slotProps={{
                popper: {
                  sx: {
                    zIndex: Z_INDEX.DRAWER_POPUP(theme),
                  },
                },
              }}
              getOptionLabel={getColumnDisplayName}
              value={selectedColumn}
              onChange={(_e, val) => setSelectedColumn(val)}
              renderOption={(props, option) => {
                const isDynamicDimension =
                  option.column.startsWith(DIMENSION_PREFIX);
                return (
                  <li {...props} key={option.column}>
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        width: '100%',
                      }}
                    >
                      <span>{getColumnDisplayName(option)}</span>
                      {isDynamicDimension && <DimBadge />}
                    </Box>
                  </li>
                );
              }}
              renderInput={(params) => (
                <TextField {...params} label="Select Dimension" size="small" />
              )}
            />
          </Box>

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Typography
              variant="caption"
              sx={{
                fontWeight: 700,
                color: 'text.secondary',
                textTransform: 'uppercase',
                letterSpacing: 1,
              }}
            >
              Step 2: Limit Count
            </Typography>
            <TextField
              label="Limit Count"
              type="number"
              size="small"
              value={limitCount}
              onChange={(e) =>
                setLimitCount(
                  Math.max(1, Math.min(500, Number(e.target.value))),
                )
              }
              inputProps={{ min: 1, max: 500 }}
              helperText="Maximum number of series to generate per chart for this split."
            />
          </Box>
        </Box>

        {/* Actions Footer */}
        <Box
          sx={{
            p: 2.5,
            borderTop: '1px solid',
            borderColor: 'divider',
            display: 'flex',
            justifyContent: 'flex-end',
            gap: 1.5,
            bgcolor: 'background.paper',
          }}
        >
          <Button onClick={() => setOpen(false)} color="inherit" sx={{ px: 3 }}>
            Cancel
          </Button>
          <Button
            onClick={handleAdd}
            variant="contained"
            disabled={!selectedColumn}
            sx={{
              px: { xs: 3, sm: 4 },
              minWidth: { sm: 140 },
            }}
          >
            Add
          </Button>
        </Box>
      </Drawer>
    </Box>
  );
}
