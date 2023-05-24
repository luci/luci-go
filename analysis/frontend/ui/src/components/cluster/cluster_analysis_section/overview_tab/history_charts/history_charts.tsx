// Copyright 2023 The LUCI Authors.
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

import './style.css';

import {
  useContext,
  useState,
} from 'react';
import { Link as RouterLink } from 'react-router-dom';
import {
  Bar,
  BarChart,
  LabelList,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import Checkbox from '@mui/material/Checkbox';
import CircularProgress from '@mui/material/CircularProgress';
import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import Link from '@mui/material/Link';
import ListItemText from '@mui/material/ListItemText';
import MenuItem from '@mui/material/MenuItem';
import OutlinedInput from '@mui/material/OutlinedInput';
import Select, { SelectChangeEvent } from '@mui/material/Select';
import Switch from '@mui/material/Switch';
import Typography from '@mui/material/Typography';

import { ClusterContext } from '@/components/cluster/cluster_context';
import PanelHeading from '@/components/headings/panel_heading/panel_heading';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import useFetchMetrics from '@/hooks/use_fetch_metrics';
import useQueryClusterHistory from '@/hooks/use_query_cluster_history';
import { Metric } from '@/services/metrics';
import { getMetricColor } from '@/tools/metric_colors';

const ITEM_HEIGHT = 48;
const ITEM_PADDING_TOP = 8;
const MenuProps = {
  PaperProps: {
    style: {
      maxHeight: ITEM_HEIGHT * 4.5 + ITEM_PADDING_TOP,
      width: 250,
    },
  },
};

interface SingleMetricChartProps {
  chartsCount: number,
  daysCount: number
  color: string,
  isAnnotated: boolean,
  metric: Metric,
  data: any[],
}

const SingleMetricChart = ({ chartsCount, daysCount, color, isAnnotated, metric, data }: SingleMetricChartProps) => {
  // Calculate the relative minimum width of the chart based on the
  // number of days (90 days is the max).
  const chartMinWidth = (daysCount / 90) * 100;

  // Reduce chart height if all charts don't fit in 1 row.
  const chartHeight = chartMinWidth * chartsCount > 100 ? 200 : 400;

  return (
    <div
      data-testid={'chart-' + metric.metricId}
      className="history-chart"
      style={{ minWidth: `${chartMinWidth}%` }} >
      <ResponsiveContainer
        width="100%"
        height={chartHeight} >
        <BarChart
          data={data}
          syncId="impactMetrics"
          margin={{ top: 20, bottom: 20 }} >
          <XAxis dataKey="date" />
          <YAxis />
          <Legend />
          <Tooltip />
          <Bar
            name={metric.humanReadableName}
            dataKey={`metrics.${metric.metricId}`}
            fill={color}>
            {isAnnotated && (
              <LabelList dataKey={`metrics.${metric.metricId}`} position="top" />
            )}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

export const HistoryCharts = () => {
  const clusterId = useContext(ClusterContext);
  // TODO: move days and selectedMetrics into the URL.
  const [days, setDays] = useState(7);
  // The values will not be annotated by default.
  const [isAnnotated, setIsAnnotated] = useState(false);

  const [selectedMetrics, setSelectedMetrics] = useState<Metric[]>([]);

  const onMetricsSuccess = (metrics: Metric[]) => {
    if (!selectedMetrics.length) {
      const defaultMetrics = metrics?.filter((m) => m.isDefault);
      setSelectedMetrics(defaultMetrics);
    }
  };

  const {
    isLoading: isMetricsLoading,
    isSuccess: isMetricsSuccess,
    data: metrics,
    error: metricError,
  } = useFetchMetrics(onMetricsSuccess);

  // Note that querying the history of a single cluster is faster and cheaper.
  const {
    isLoading,
    isSuccess,
    data,
    error,
  } = useQueryClusterHistory(clusterId.project, `cluster_algorithm="${clusterId.algorithm}" cluster_id="${clusterId.id}"`, days, selectedMetrics.map((m) => m.metricId));

  const handleMetricChange = (event: SelectChangeEvent<string[]>) => {
    const {
      target: { value },
    } = event;
    // On autofill we get a stringified value.
    const selectedMetricIds = typeof value === 'string' ? value.split(',') : value;
    const selectedMetrics = (metrics || []).filter((m) => selectedMetricIds.indexOf(m.metricId) > -1);
    setSelectedMetrics(selectedMetrics);
  };

  const handleAnnotationsChange = () => {
    setIsAnnotated(!isAnnotated);
  };
  const renderValue = (selected: string[]): string => {
    return (metrics || []).filter((m) => selected.indexOf(m.metricId) >= 0)
      .map((m) => m.humanReadableName).join(', ');
  };

  return (
    <div>
      <div className="history-charts-toolbar">
        <PanelHeading>History</PanelHeading>
        <Typography color="GrayText">All dates and times are in UTC.</Typography>
        <div style={{ flexGrow: 1 }}></div>
        <FormControlLabel
          control={
            <Switch checked={isAnnotated} onChange={handleAnnotationsChange} />
          }
          label="Annotate values"
          labelPlacement="end"
        />
        <FormControl sx={{ m: 1, width: 300 }}>
          <InputLabel id="date-range-label">Date Range</InputLabel>
          <Select
            labelId="date-range-label"
            value={days}
            label="Date Range"
            onChange={(e) => setDays(e.target.value as number)}
          >
            <MenuItem value={7}>7 days</MenuItem>
            <MenuItem value={30}>30 days</MenuItem>
            <MenuItem value={90}>90 days</MenuItem>
          </Select>
        </FormControl>
        <FormControl sx={{ m: 1, width: 300 }}>
          <InputLabel id="metric-label">Metrics</InputLabel>
          <Select
            labelId="metric-label"
            multiple
            value={selectedMetrics.map((m) => m.metricId)}
            onChange={handleMetricChange}
            input={<OutlinedInput label="Name" />}
            renderValue={renderValue}
            MenuProps={MenuProps}
          >
            {(metrics || []).map((metric) => {
              return <MenuItem key={metric.metricId} value={metric.metricId}>
                <Checkbox checked={selectedMetrics.indexOf(metric) > -1} />
                <ListItemText primary={metric.humanReadableName} />
              </MenuItem>;
            })}
          </Select>
        </FormControl>
      </div>
      {(metricError) && (
        <LoadErrorAlert entityName="metrics" error={metricError} />
      )}
      {error && (
        <LoadErrorAlert entityName="cluster history" error={error} />
      )}
      {!(error || metricError) && (isLoading || isMetricsLoading) && (
        <Grid container item alignItems="center" justifyContent="center">
          <CircularProgress />
        </Grid>
      )}
      {isSuccess && isMetricsSuccess && data && metrics && (
        <div
          className="history-charts-container"
          data-testid="history-chart"
        >
          {selectedMetrics.length > 0 ?
            selectedMetrics.map((m) =>
              <SingleMetricChart
                key={m.metricId}
                chartsCount={selectedMetrics.length || 0}
                daysCount={days}
                color={getMetricColor(metrics.indexOf(m))}
                isAnnotated={isAnnotated}
                metric={m}
                data={data.days}
              />
            ) :
            <Typography color="GrayText">Select some metrics to see its history.</Typography>
          }
        </div>
      )}
      <div style={{ paddingTop: '2rem' }}>
        {selectedMetrics.length > 0 && (
          <Typography>This chart shows the history of metrics for this cluster for each day in the selected time period.</Typography>
        )}
        <Typography>To see examples of failures in this cluster, view <Link component={RouterLink} to='#recent-failures'>Recent Failures</Link>.</Typography>
      </div>
    </div>
  );
};
