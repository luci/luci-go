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

import DoneIcon from '@mui/icons-material/Done';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';

import { colors } from '@/fleet/theme/colors';
import {
  RepairMetric,
  RepairMetric_Priority,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const getPriorityIcon = (priority: RepairMetric_Priority) => {
  switch (priority) {
    case RepairMetric_Priority.NICE:
      return <DoneIcon sx={{ color: colors.green[400], width: '20px' }} />;
    case RepairMetric_Priority.MISSING_DATA:
    case RepairMetric_Priority.DEVICES_REMOVED:
    case RepairMetric_Priority.WATCH:
      return <WarningIcon sx={{ color: colors.yellow[900], width: '20px' }} />;
    case RepairMetric_Priority.BREACHED:
      return <ErrorIcon sx={{ color: colors.red[500], width: '20px' }} />;
  }
};

// mapping to snake case is needed to send the right sort "by" to the backend
export const getRow = (rm: RepairMetric) => ({
  id: rm.labName + rm.hostGroup + rm.runTarget,
  priority: rm.priority,
  lab_name: rm.labName,
  host_group: rm.hostGroup,
  run_target: rm.runTarget,
  minimum_repairs: rm.minimumRepairs,
  devices_offline_ratio: `${rm.devicesOffline} / ${rm.totalDevices}`,
  devices_offline_percentage: (
    rm.devicesOffline / rm.totalDevices
  ).toLocaleString('en-GB', {
    style: 'percent',
    minimumFractionDigits: 1,
  }),
  peak_usage: rm.peakUsage,
  total_devices: rm.totalDevices,
});

export type Row = ReturnType<typeof getRow>;
