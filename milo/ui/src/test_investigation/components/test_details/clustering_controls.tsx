// Copyright 2025 The LUCI Authors.
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
  Box,
  FormControl,
  MenuItem,
  Select,
  SelectChangeEvent,
  Typography,
} from '@mui/material';

import {
  TestResult,
  TestResult_Status,
  testResult_StatusToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { ClusteredResult } from './types';

interface ClusteringControlsProps {
  clusteredFailures: readonly ClusteredResult[];
  selectedClusterIndex: number;
  onClusterChange: (event: SelectChangeEvent<number>) => void;
  currentAttempts: readonly TestResultBundle[];
  selectedAttemptIndex: number;
  onAttemptChange: (event: SelectChangeEvent<number>) => void;
  currentResult?: TestResult;
  currentCluster?: ClusteredResult;
}

function getResultStatusV2DisplayText(statusV2?: TestResult_Status): string {
  if (statusV2 === undefined) return 'N/A';
  switch (statusV2) {
    case TestResult_Status.PASSED:
      return 'Passed';
    case TestResult_Status.FAILED:
      return 'Failed';
    case TestResult_Status.SKIPPED:
      return 'Skipped';
    case TestResult_Status.EXECUTION_ERRORED:
      return 'Execution Error';
    case TestResult_Status.PRECLUDED:
      return 'Precluded';
    case TestResult_Status.STATUS_UNSPECIFIED:
      return 'Unspecified';
    default:
      return testResult_StatusToJSON(statusV2) || 'Unknown';
  }
}

export function ClusteringControls({
  clusteredFailures,
  selectedClusterIndex,
  onClusterChange,
  currentAttempts,
  selectedAttemptIndex,
  onAttemptChange,
  currentResult,
  currentCluster,
}: ClusteringControlsProps) {
  if (!currentCluster) {
    // No cluster means no controls to show
    return null;
  }

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 2,
        mb: 2,
        flexWrap: 'wrap',
      }}
    >
      <FormControl variant="standard" size="small" sx={{ minWidth: 180 }}>
        <Select
          value={selectedClusterIndex}
          onChange={onClusterChange}
          aria-label="Select Failure Cluster"
          renderValue={(value) =>
            // Use statusV2 from the first attempt's result in the current cluster
            `${getResultStatusV2DisplayText(currentAttempts[0]?.result?.statusV2)}: Cluster ${value + 1} of ${clusteredFailures.length}`
          }
          sx={{
            fontSize: '0.875rem',
            '.MuiSelect-icon': { fontSize: '1.2rem' },
          }}
        >
          {clusteredFailures.map((cluster, index) => (
            <MenuItem key={cluster.clusterKey + index} value={index}>
              Cluster {index + 1}: {cluster.clusterKey.substring(0, 50)}
              {cluster.clusterKey.length > 50 ? '...' : ''} (
              {cluster.results.length} attempts)
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <FormControl variant="standard" size="small" sx={{ minWidth: 180 }}>
        <Select
          value={selectedAttemptIndex}
          onChange={onAttemptChange}
          aria-label="Select Attempt"
          renderValue={(value) =>
            // Use statusV2 from the current selected result
            `${getResultStatusV2DisplayText(currentResult?.statusV2)}: Attempt ${value + 1} of ${currentAttempts.length}`
          }
          sx={{
            fontSize: '0.875rem',
            '.MuiSelect-icon': { fontSize: '1.2rem' },
          }}
          disabled={currentAttempts.length <= 1}
        >
          {currentAttempts.map((_attemptBundle, index) => (
            <MenuItem key={index} value={index}>
              Attempt {index + 1}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{
          flexGrow: 1,
          minWidth: '200px',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}
      >
        {currentCluster.originalFailureReason}
      </Typography>
    </Box>
  );
}
