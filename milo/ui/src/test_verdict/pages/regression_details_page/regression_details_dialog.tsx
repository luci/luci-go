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
import { Box, Dialog, DialogContent, DialogTitle, Link } from '@mui/material';
import { useRef } from 'react';

import { QueuedStickyScrollingBase } from '@/generic_libs/components/queued_sticky';
import { getGitilesCommitURL } from '@/gitiles/tools/utils';
import { BlamelistTable } from '@/test_verdict/components/blamelist_table';

import { useBlamelistDispatch, useBlamelistState } from './context';

export function RegressionDetailsDialog() {
  const dispatch = useBlamelistDispatch();
  const state = useBlamelistState();
  // TODO: implement scroll to focus commit position.
  const testVariantBranch = state.testVariantBranch;

  const scrollRef = useRef<HTMLDivElement>(null);

  if (!testVariantBranch) {
    return <></>;
  }

  return (
    <Dialog
      open
      onClose={() => dispatch({ type: 'dismiss' })}
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
      <DialogTitle
        sx={{
          padding: 0,
          display: 'grid',
          gridTemplateColumns: '1fr 24px',
          borderBottom: 'solid 1px var(--divider-color)',
          backgroundColor: 'var(--block-background-color)',
          fontSize: '16px',
        }}
      >
        <table>
          <tbody>
            <tr>
              <td width="1px">Branch:</td>
              <td css={{ fontWeight: 400 }}>
                <Link href={getGitilesCommitURL(testVariantBranch.ref.gitiles)}>
                  {testVariantBranch.ref.gitiles.ref}
                </Link>
              </td>
            </tr>
            <tr>
              <td width="1px">Test ID:</td>
              <td css={{ fontWeight: 400 }}>{testVariantBranch.testId}</td>
            </tr>
            <tr>
              <td width="1px">Variant:</td>
              <td css={{ fontWeight: 400 }}>
                {JSON.stringify(testVariantBranch.variant?.def || {})}
              </td>
            </tr>
          </tbody>
        </table>
        <Box
          title="press esc to close the blamelist overlay"
          onClick={() => dispatch({ type: 'dismiss' })}
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
      </DialogTitle>
      <QueuedStickyScrollingBase
        sx={{ padding: 0 }}
        component={DialogContent}
        ref={scrollRef}
      >
        <BlamelistTable
          testVariantBranch={testVariantBranch}
          customScrollParent={scrollRef.current || undefined}
        />
      </QueuedStickyScrollingBase>
    </Dialog>
  );
}
