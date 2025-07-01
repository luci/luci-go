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
  Typography,
  SelectChangeEvent,
} from '@mui/material';

import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { StatusKindChip } from './status_kind_chip';
import { ClusteredResult } from './types';

interface ClusteringControlsProps {
  clusteredFailures: readonly ClusteredResult[];
  selectedClusterIndex: number;
  onClusterChange: (event: SelectChangeEvent<number>) => void;
  currentAttempts: readonly TestResultBundle[];
  selectedAttemptIndex: number;
  onAttemptChange: (event: SelectChangeEvent<number>) => void;
  currentCluster?: ClusteredResult;
}

interface ClusterMenuItemContentProps {
  cluster: ClusteredResult;
  index: number;
}

function ClusterMenuItemContent({
  cluster,
  index,
}: ClusterMenuItemContentProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-start',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
        <Typography variant="body2" sx={{ fontWeight: 'medium' }}>
          Cluster {index + 1}
        </Typography>
        <StatusKindChip
          statusV2={cluster.results[0]?.result?.statusV2}
          failureKindKeyPart={cluster.failureKindKeyPart}
          skippedKindKeyPart={cluster.skippedKindKeyPart}
        />
      </Box>
      <Typography
        variant="caption"
        sx={{ color: 'text.secondary', textAlign: 'left' }}
      >
        {cluster.originalFailureReason && (
          <>Reason: {cluster.originalFailureReason.substring(0, 140)}</>
        )}
        {cluster.originalFailureReason.length > 140 ? '...' : ''}
      </Typography>
      <Typography
        variant="caption"
        sx={{ color: 'text.secondary', textAlign: 'left' }}
      >
        ({cluster.results.length} attempt
        {cluster.results.length > 1 ? 's' : ''})
      </Typography>
    </Box>
  );
}

export function ClusteringControls({
  clusteredFailures,
  selectedClusterIndex,
  onClusterChange,
  currentAttempts,
  selectedAttemptIndex,
  onAttemptChange,
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
        flexWrap: 'wrap',
      }}
    >
      <FormControl variant="standard" size="small" sx={{ minWidth: 180 }}>
        <Select
          value={selectedClusterIndex}
          onChange={onClusterChange}
          aria-label="Select Failure Cluster"
          variant="outlined"
          renderValue={(value) => (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <StatusKindChip
                statusV2={currentAttempts[0]?.result?.statusV2}
                failureKindKeyPart={currentCluster.failureKindKeyPart}
                skippedKindKeyPart={currentCluster.skippedKindKeyPart}
              />
              {` Cluster ${value + 1} of ${clusteredFailures.length}`}
            </Box>
          )}
          sx={{
            fontSize: '0.875rem',
          }}
        >
          {clusteredFailures.map((cluster, index) => (
            <MenuItem
              key={cluster.clusterKey + index}
              value={index}
              sx={{ display: 'block' }}
            >
              <ClusterMenuItemContent cluster={cluster} index={index} />
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <FormControl variant="standard" size="small" sx={{ minWidth: 180 }}>
        <Select
          value={selectedAttemptIndex}
          onChange={onAttemptChange}
          aria-label="Select Attempt"
          variant="outlined"
          renderValue={(value) =>
            `Attempt ${value + 1} of ${currentAttempts.length}`
          }
          sx={{
            fontSize: '0.875rem',
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
    </Box>
  );
}
