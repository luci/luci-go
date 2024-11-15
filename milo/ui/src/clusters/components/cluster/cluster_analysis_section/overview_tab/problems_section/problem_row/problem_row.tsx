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

import Chip from '@mui/material/Chip';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { DateTime } from 'luxon';
import { useContext } from 'react';

import { Problem } from '@/clusters/tools/problems';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuartion } from '@/common/tools/time_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { OverviewTabContextData } from '../../overview_tab_context';
import { PriorityChip } from '../priority_chip/priority_chip';
import { StatusChip } from '../status_chip/status_chip';

interface Props {
  problem: Problem;
  openProblemDialog: (p: Problem) => void;
}

export const ProblemRow = ({ problem, openProblemDialog }: Props) => {
  const { metrics } = useContext(OverviewTabContextData);
  const [, setSearchParams] = useSyncedSearchParams();

  const problemMetricID = problem.policy.metrics[0].metricId;

  const handleOpenClicked = () => {
    openProblemDialog(problem);
  };
  const handleShowFailures = () => {
    // TODO: The data model allows each policy to have multiple metrics.
    // At time of implementation, nobody is using this feature so we are not
    // bothering to supporting it here. In future, we should update the button
    // to display a dropdown so the user can select which metric they
    // want to drill into.
    setSearchParams((params) => {
      params.set('tab', 'recent-failures');
      params.set('filterToMetric', problemMetricID);
      return params;
    });
  };
  return (
    <TableRow
      key={problem.policy.id}
      data-testid={'policy-row-' + problem.policy.id}
    >
      <TableCell
        sx={{
          fontSize: '1rem',
          paddingLeft: '0px',
          textDecoration: problem.state.isActive ? 'none' : 'line-through',
        }}
      >
        {problem.policy.humanReadableName}
      </TableCell>
      <TableCell sx={{ fontSize: '1rem' }}>
        <StatusChip isActive={problem.state.isActive} />
      </TableCell>
      <TableCell sx={{ fontSize: '1rem' }}>
        {problem.state.isActive ? (
          <PriorityChip priority={problem.policy.priority} />
        ) : (
          '-'
        )}
      </TableCell>
      <TableCell sx={{ fontSize: '1rem' }}>
        {problem.state.isActive ? (
          <RelativeTimestamp
            formatFn={displayApproxDuartion}
            timestamp={DateTime.fromISO(problem.state.lastActivationTime || '')}
          />
        ) : (
          '-'
        )}
      </TableCell>
      <TableCell>
        <Chip
          aria-label="View details of problems"
          variant="outlined"
          color="default"
          onClick={handleOpenClicked}
          label={<Typography variant="button">more info</Typography>}
          sx={{ borderRadius: 1, marginRight: '0.5rem' }}
        />

        <Tooltip
          title={
            <>
              View recent failures, filtered to those which contribute to the
              problem metric,{' '}
              <strong>
                {metrics.find((m) => m.metricId === problemMetricID)
                  ?.humanReadableName || '(unavailable)'}
              </strong>
              .
            </>
          }
        >
          <Chip
            aria-label="View failures related to problem"
            variant="outlined"
            color="default"
            onClick={handleShowFailures}
            label={<Typography variant="button">show failures</Typography>}
            sx={{ borderRadius: 1 }}
          />
        </Tooltip>
      </TableCell>
    </TableRow>
  );
};
