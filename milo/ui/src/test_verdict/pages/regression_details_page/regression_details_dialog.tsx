// Copyright 2024 The LUCI Authors.
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

import { Close } from '@mui/icons-material';
import {
  Box,
  CircularProgress,
  Dialog,
  DialogContent,
  Link,
  styled,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import { getGitilesCommitURL } from '@/gitiles/tools/utils';
import { Changepoint } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { QuerySourcePositionsRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { CommitRowEntry } from './commit_row_entry';

const Entry = styled('div')({
  svg: {
    height: '24px',
    width: '24px',
  },
});

export interface RegressionDetailsDialogProps {
  readonly changepoint: Changepoint;
}

export function RegressionDetailsDialog({
  changepoint,
}: RegressionDetailsDialogProps) {
  const [showOverlay, setShowOverlay] = useState(true);
  const client = useTestVariantBranchesClient();
  const {
    project,
    testId,
    variantHash,
    variant,
    ref,
    refHash,
    nominalStartPosition,
    startPositionLowerBound99th,
    startPositionUpperBound99th,
  } = changepoint;
  // TODO: Add pagination.
  const { data, isLoading, isError, error } = useQuery(
    client.QuerySourcePositions.query(
      QuerySourcePositionsRequest.fromPartial({
        project,
        testId,
        variantHash,
        refHash,
        startSourcePosition: nominalStartPosition,
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }
  const failureStartIdx =
    2 +
    data.sourcePositions.findIndex(
      (sp) => sp.position === nominalStartPosition,
    );
  const regressionUpperIdx =
    1 +
    data.sourcePositions.findIndex(
      (sp) => sp.position === startPositionUpperBound99th,
    );
  const lastLine = data.sourcePositions.length + 1;
  const regressionLowerIndex =
    1 +
      data.sourcePositions.findIndex(
        (sp) => sp.position === startPositionLowerBound99th,
      ) || lastLine;
  return (
    <Dialog
      open={showOverlay}
      onClose={() => setShowOverlay(false)}
      maxWidth={false}
      fullWidth
      disableScrollLock
      sx={{
        '& .MuiDialog-paper': {
          margin: 0,
          maxWidth: '100%',
          width: '100%',
          maxHeight: '60%',
          height: '60%',
          position: 'absolute',
          bottom: 0,
          borderRadius: 0,
        },
      }}
    >
      <DialogContent sx={{ padding: 0 }}>
        <Box
          sx={{
            lineHeight: '20px',
            backgroundColor: 'var(--block-background-color)',
            borderTop: '1px solid var(--divider-color)',
            paddingLeft: '4px',
          }}
        >
          <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 24px' }}>
            <Box>
              <table>
                <tbody>
                  <tr>
                    <td>Branch:</td>
                    <td css={{ fontWeight: 400 }}>
                      <Link href={getGitilesCommitURL(ref!.gitiles!)}>
                        {ref?.gitiles?.ref}
                      </Link>
                    </td>
                  </tr>
                  <tr>
                    <td>Test ID:</td>
                    <td css={{ fontWeight: 400 }}>{testId}</td>
                  </tr>
                  <tr>
                    <td>Variant:</td>
                    <td css={{ fontWeight: 400 }}>
                      {JSON.stringify(variant?.def)}
                    </td>
                  </tr>
                </tbody>
              </table>
            </Box>
            <Box
              title="press esc to close the test variant details table"
              onClick={() => setShowOverlay(false)}
            >
              <Close
                css={{
                  color: 'red',
                  cursor: 'pointer',
                  verticalAlign: 'bottom',
                  paddingBottom: '4px',
                }}
              />
            </Box>
          </Box>
        </Box>
        <Box
          sx={{
            borderTop: '1px solid var(--divider-color)',
            borderBottom: '1px solid var(--divider-color)',
            '--commit-columns': '24px 40px 100px 200px 300px 300px 1fr',
          }}
        >
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: '75px var(--commit-columns)',
              gridGap: '5px',
              lineHeight: '30px',
              fontWeight: 'bold',
              backgroundColor: 'var(--block-background-color)',
              borderTop: '1px solid var(--divider-color)',
              borderBottom: '1px solid var(--divider-color)',
            }}
          >
            <Box>Segments</Box>
            <Box></Box>
            <Box>Verd.</Box>
            <Box>Commit</Box>
            <Box>Time</Box>
            <Box>Author</Box>
            <Box>Title</Box>
          </Box>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '30px 30px 1fr',
              gridGap: '5px',
              fontSize: '16px',
              marginLeft: '10px',
            }}
          >
            <Box
              sx={{
                width: '24px',
                height: '100%',
                backgroundColor: 'var(--success-bg-color)',
                border: '1px solid var(--success-color)',
                gridColumnStart: 1,
                gridRowStart: failureStartIdx,
                gridRowEnd: lastLine,
              }}
            ></Box>
            <Box
              sx={{
                width: '24px',
                height: '100%',
                border: '1px solid var(--failure-color)',
                backgroundColor: 'var(--failure-bg-color)',
                gridColumnStart: 1,
                gridRowStart: 1,
                gridRowEnd: failureStartIdx,
              }}
            ></Box>
            <Box
              sx={{
                width: '24px',
                height: '100%',
                border: '1px solid var(--canceled-color)',
                backgroundColor: 'var(--canceled-bg-color)',
                gridRowStart: regressionUpperIdx,
                gridRowEnd: regressionLowerIndex,
                gridColumnStart: 2,
              }}
            ></Box>
            {data.sourcePositions.map((sp) => (
              <Entry key={sp.position} style={{ gridColumnStart: 3 }}>
                <CommitRowEntry sourcePosition={sp} sourceRef={ref} />
              </Entry>
            ))}
          </div>
        </Box>
      </DialogContent>
    </Dialog>
  );
}
