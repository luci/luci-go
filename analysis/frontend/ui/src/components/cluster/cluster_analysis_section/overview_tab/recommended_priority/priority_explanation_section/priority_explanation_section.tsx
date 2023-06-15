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

import CloseIcon from '@mui/icons-material/Close';
import DoneIcon from '@mui/icons-material/Done';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import {
  Criterium,
  PriorityCriteriaResult,
  PriorityRecommendation,
} from '../priority_recommendation';

export function constructCriteriumLabel(c: Criterium): string {
  return `${c.metricName} (${c.durationKey}) (value: ${c.actualValue}) \u2265 ${c.thresholdValue}`;
}

const NoPriorityCriteriaAlert = () => {
  return (
    <Box>
      <Alert severity="warning">
        A priority has not been recommended because the project configuration does not have priority thresholds specified.
      </Alert>
    </Box>
  );
};

const PriorityRecommendationDescription = () => {
  return <>
    <Typography>
      To get the bug priority recommendation, the cluster's impact was compared to the thresholds specified in the <Link
        href="http://goto.google.com/luci-analysis-setup#project-configuration"
        target="_blank"
        rel="noreferrer"
        underline="always">
        project configuration</Link>. Note:
    </Typography>
    <ul>
      <li>
        <Typography>
          Even if a LUCI Analysis rule is configured to control the bug priority, the priority shown here may not match the bug due to <Link
            href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=hysteresis"
            target="_blank"
            rel="noreferrer"
            underline="always">hysteresis</Link>.
        </Typography>
      </li>
      <li>
        <Typography>
          Even if a cluster has enough impact to have a priority, a bug may not be filed for it if it does not meet the  <Link
            href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=bug_filing_threshold"
            target="_blank"
            rel="noreferrer"
            underline="always">auto bug-filing threshold</Link>.
        </Typography>
      </li>
      <li>
        <Typography>
          Higher priorities can only be reached if the thresholds for all lower priorities have also been met.
        </Typography>
      </li>
    </ul>
    <Typography>
      See <Link
        href="http://goto.google.com/luci-analysis-bug-management"
        target="_blank"
        rel="noreferrer"
        underline="always">go/luci-analysis-bug-management</Link> for more information.
    </Typography>
  </>;
};

interface PriorityRowProps {
  priorityResult: PriorityCriteriaResult;
}

const PriorityRow = ({ priorityResult }: PriorityRowProps) => {
  const elements = [];

  // Construct the list of elements for each metric criterium, inserting an
  // element to display "OR" between criteria.
  for (let i = 0; i < priorityResult.criteria.length; i++) {
    const criterium = priorityResult.criteria[i];
    const criteriumElement = (
      <Box
        key={`${priorityResult.priority}:${criterium.metricName}`}
        className="priority-criterium" >
        <Chip
          variant="outlined"
          icon={criterium.satisfied ? (<DoneIcon />) : (<CloseIcon />)}
          color={criterium.satisfied ? 'success' : 'default'}
          label={constructCriteriumLabel(criterium)} />
        {(i < priorityResult.criteria.length - 1) &&
          <Typography
            component="span"
            sx={{ paddingLeft: '0.5rem' }} >
            OR
          </Typography>
        }
      </Box>
    );
    elements.push(criteriumElement);
  }

  return (
    <TableRow data-testid="priority-explanation-table-row">
      <TableCell>
        {priorityResult.priority}
      </TableCell>
      <TableCell>
        {elements}
      </TableCell>
      <TableCell data-testid="priority-explanation-table-row-verdict">
        {priorityResult.satisfied ?
          <Typography display="flex">
            <DoneIcon color="success" sx={{ marginRight: '0.5rem' }} />
            Yes
          </Typography> :
          <Typography display="flex">
            <CloseIcon color="error" sx={{ marginRight: '0.5rem' }} />
            No
          </Typography>
        }
      </TableCell>
    </TableRow>
  );
};

interface Props {
  recommendation: PriorityRecommendation;
}

const PriorityJustification = ({ recommendation }: Props) => {
  return (
    <>
      <Table
        size="small"
        data-testid="priority-explanation-table">
        <TableHead>
          <TableRow>
            <TableCell>Priority</TableCell>
            <TableCell>Criteria</TableCell>
            <TableCell>Satisfied?</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {recommendation.justification.map((result) =>
            <PriorityRow
              key={result.priority}
              priorityResult={result}></PriorityRow>,
          )}
        </TableBody>
      </Table>
      {!recommendation.recommendation && (
        <Box paddingTop={3}>
          <Alert severity="info">
            A priority was not recommended because no priority's criteria was satisfied.
          </Alert>
        </Box>
      )}
    </>
  );
};

export const PriorityExplanationSection = ({ recommendation }: Props) => {
  return (
    <main data-testid="priority-explanation-section">
      <PriorityRecommendationDescription />
      <Box paddingTop={3}>
        {recommendation.justification.length > 0 ?
          <PriorityJustification recommendation={recommendation} /> :
          <NoPriorityCriteriaAlert />
        }
      </Box>
    </main>
  );
};
