// Copyright 2022 The LUCI Authors.
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

import { useContext, useState } from 'react';
import {
  Link as RouterLink,
} from 'react-router-dom';

import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import TabPanel from '@mui/lab/TabPanel';
import Typography from '@mui/material/Typography';
import {
  Checkbox,
  FormControl,
  InputLabel,
  ListItemText,
  MenuItem,
  OutlinedInput,
  Select,
  SelectChangeEvent,
} from '@mui/material';
import {
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  BarChart,
  Bar,
  ResponsiveContainer,
} from 'recharts';

import PanelHeading from '@/components/headings/panel_heading/panel_heading';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import { ClusterContext } from '../../cluster_context';
import useQueryClusterHistory from '@/hooks/use_query_cluster_history';
import useFetchMetrics from '@/hooks/use_fetch_metrics';
import { Metric } from '@/services/metrics';

import './style.css';

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

interface Props {
  // The name of the tab.
  value: string;
}

const metricColors = {
  'human-cls-failed-presubmit': "#6c40bf",
  'critical-failures-exonerated': "#0084ff",
  'test-runs-failed': "#d23a2d",
}
const metricIds = ['human-cls-failed-presubmit', 'critical-failures-exonerated', 'test-runs-failed'];
const OverviewTab = ({
  value,
}: Props) => {
  const clusterId = useContext(ClusterContext);
  // TODO: move days and selectedMetrics into the URL.
  const [days, setDays] = useState(7);
  const [selectedMetrics, setSelectedMetrics] = useState([...metricIds]);

  // FIXME: normally we fix this up on the server where we have access to the
  // latest version number.  Is there a way to do the same in the client?
  const algorithm = clusterId.algorithm == 'rules' ? 'rules-v2' : clusterId.algorithm;

  // Note that querying the history of a single cluster is faster and cheaper.
  const {
    isLoading,
    isSuccess,
    data,
    error,
  } = useQueryClusterHistory(clusterId.project, `cluster_algorithm="${algorithm}" cluster_id="${clusterId.id}"`, days, metricIds);

  const fetchedMetrics = useFetchMetrics();
  const metric = (metricId: string): Metric | undefined =>
    fetchedMetrics?.data?.filter(m => m.metricId == metricId)?.[0];

  const handleMetricChange = (event: SelectChangeEvent<typeof selectedMetrics>) => {
    const {
      target: { value },
    } = event;
    setSelectedMetrics(
      // On autofill we get a stringified value.
      typeof value === 'string' ? value.split(',') : value,
    );
  };

  return (
    <TabPanel value={value}>
      <div className="overview-tab-toolbar">
        <PanelHeading>History</PanelHeading>
        <Typography color="GrayText">All dates and times are in UTC.</Typography>
        <div style={{ flexGrow: 1 }}></div>
        <FormControl sx={{ m: 1, width: 300 }}>
          <InputLabel id="date-range-label">Date Range</InputLabel>
          <Select
            labelId="date-range-label"
            value={days}
            label="Date Range"
            onChange={e => setDays(e.target.value as number)}
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
            value={selectedMetrics}
            onChange={handleMetricChange}
            input={<OutlinedInput label="Name" />}
            renderValue={(selected) => selected.map(m => metric(m)?.humanReadableName || m).join(', ')}
            MenuProps={MenuProps}
          >
            {metricIds.map(m => {
              return <MenuItem
                key={m}
                value={m}
              >
                <Checkbox checked={selectedMetrics.indexOf(m) > -1} />
                <ListItemText primary={metric(m)?.humanReadableName || m} />
              </MenuItem>
            })}
          </Select>
        </FormControl>
      </div>
      {
        isLoading && (
          <Grid container item alignItems="center" justifyContent="center">
            <CircularProgress />
          </Grid>
        )
      }
      {
        !isLoading && error && (
          <LoadErrorAlert
            entityName="metrics"
            error={error}
          />
        )
      }
      {
        isSuccess && data && (
          <div data-testid="history-chart">
            <ResponsiveContainer width="100%" height={400}>
              <BarChart data={data.days} margin={{ top: 20, bottom: 20 }}>
                <XAxis dataKey="date" />
                <YAxis />
                <Legend />
                <Tooltip />
                {selectedMetrics.map(m => {
                  const mk = m as keyof (typeof metricColors);
                  return <Bar key={m} name={metric(m)?.humanReadableName || m} dataKey={`metrics.${m}`} fill={metricColors[mk]} />;
                })}
              </BarChart>
            </ResponsiveContainer>
          </div>
        )
      }
      <Typography paddingTop='2rem'>This chart shows the history of metrics for this cluster for each day in the selected time period.</Typography>
      <Typography>To see examples of failures in this cluster, view <Link component={RouterLink} to='#recent-failures'>Recent Failures</Link>.</Typography>
    </TabPanel>
  );
};

export default OverviewTab;
