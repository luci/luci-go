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
  Delete as DeleteIcon,
  ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Button,
  Chip,
  TextField,
  Typography,
  IconButton,
} from '@mui/material';
import { useEffect, useState, useCallback } from 'react';

import { PerfChartSeries } from '@/crystal_ball/types';

interface ChartSeriesEditorProps {
  series: PerfChartSeries[];
  onUpdateSeries: (updatedSeries: PerfChartSeries[]) => void;
  dataSpecId: string;
}

// Helper function to get a stable ID for a series item
const getSeriesId = (s: PerfChartSeries, index: number) =>
  s.displayName || `series-${index}`;

export function ChartSeriesEditor({
  series,
  onUpdateSeries,
  dataSpecId,
}: ChartSeriesEditorProps) {
  const [expanded, setExpanded] = useState(false);
  // Local state to manage draft metric fields
  const [draftMetricFields, setDraftMetricFields] = useState<
    Record<string, string>
  >({});

  useEffect(() => {
    const initialDrafts: Record<string, string> = {};
    series.forEach((s, index) => {
      const id = getSeriesId(s, index);
      initialDrafts[id] = s.metricField || '';
    });
    setDraftMetricFields(initialDrafts);
  }, [series]);

  const handleAddSeries = () => {
    const newSeries: PerfChartSeries = {
      displayName: `series-${crypto.randomUUID()}`,
      metricField: '',
      dataSpecId: dataSpecId,
    };
    onUpdateSeries([...series, newSeries]);
  };

  const handleRemoveSeries = (index: number) => {
    const updatedSeries = [...series];
    updatedSeries.splice(index, 1);
    onUpdateSeries(updatedSeries);
  };

  const handleDraftChange = useCallback((seriesId: string, value: string) => {
    setDraftMetricFields((prev) => ({ ...prev, [seriesId]: value }));
  }, []);

  const handleDraftBlur = useCallback(
    (index: number, seriesId: string) => {
      const newMetricField = draftMetricFields[seriesId];
      const currentSeries = series[index];

      if (currentSeries && currentSeries.metricField !== newMetricField) {
        const updatedSeries = [...series];
        updatedSeries[index] = {
          ...currentSeries,
          metricField: newMetricField,
          // Optionally update displayName if it wasn't manually set
          displayName:
            currentSeries.displayName &&
            !currentSeries.displayName.startsWith('series-')
              ? currentSeries.displayName
              : newMetricField || currentSeries.displayName,
        };
        onUpdateSeries(updatedSeries);
      }
    },
    [draftMetricFields, series, onUpdateSeries],
  );

  return (
    <Box sx={{ mt: 1 }}>
      <Accordion expanded={expanded} onChange={() => setExpanded(!expanded)}>
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          aria-controls="series-content"
          id="series-header"
          sx={{
            '& .MuiAccordionSummary-content': {
              alignItems: 'center',
              gap: 1,
            },
          }}
        >
          <Typography variant="subtitle1">Series</Typography>
          {!expanded && series.length > 0 && (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {series.map((s, index) => (
                <Chip
                  key={getSeriesId(s, index)}
                  label={s.metricField || 'New Series'}
                  size="small"
                />
              ))}
            </Box>
          )}
          {!expanded && series.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              No series added.
            </Typography>
          )}
        </AccordionSummary>
        <AccordionDetails>
          {series.map((singleSeries, index) => {
            const seriesId = getSeriesId(singleSeries, index);
            return (
              <Box
                key={seriesId}
                sx={{
                  display: 'grid',
                  gridTemplateColumns: '1fr auto',
                  gap: 1,
                  alignItems: 'center',
                  mb: 1.5,
                }}
              >
                <TextField
                  placeholder="Metric Field (e.g., MemAvailable_CacheProcDirty_bytes)"
                  size="small"
                  value={draftMetricFields[seriesId] || ''}
                  onChange={(e) => handleDraftChange(seriesId, e.target.value)}
                  onBlur={() => handleDraftBlur(index, seriesId)}
                  inputProps={{ 'aria-label': 'Metric Field' }}
                />
                <IconButton
                  onClick={() => handleRemoveSeries(index)}
                  aria-label="Remove series"
                  color="error"
                  size="small"
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Box>
            );
          })}
          <Button
            startIcon={<AddIcon />}
            onClick={handleAddSeries}
            variant="outlined"
            size="small"
            sx={{ mt: 1 }}
          >
            Add Series
          </Button>
        </AccordionDetails>
      </Accordion>
    </Box>
  );
}
