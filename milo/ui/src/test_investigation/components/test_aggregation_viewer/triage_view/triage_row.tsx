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
  ExpandLess,
  ExpandMore,
  KeyboardArrowDown,
  KeyboardArrowRight,
} from '@mui/icons-material';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';

import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { TriageViewNode } from './context/context';
import { VerdictNode } from './verdict_node';

export interface TriageRowProps {
  node: TriageViewNode;
  toggleExpansion: (id: string) => void;
}

export function TriageRow({ node, toggleExpansion }: TriageRowProps) {
  if (node.type === 'status') {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          height: '100%',
          px: 1,
          bgcolor: 'action.hover',
          borderBottom: '1px solid',
          borderColor: 'divider',
          cursor: 'pointer',
          userSelect: 'none',
        }}
        onClick={() => toggleExpansion(node.id)}
      >
        <IconButton size="small" sx={{ mr: 1, p: 0.5 }}>
          {node.expanded ? (
            <ExpandLess fontSize="small" />
          ) : (
            <ExpandMore fontSize="small" />
          )}
        </IconButton>
        <Typography
          variant="subtitle2"
          sx={{
            fontWeight: 'bold',
            flexGrow: 1,
            ...getStatusStyle(node.id),
          }}
          noWrap
        >
          {node.id} ({node.group.count})
        </Typography>
      </Box>
    );
  }

  if (node.type === 'group') {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          height: '100%',
          pl: 3, // Indent
          pr: 1,
          borderBottom: '1px solid',
          borderColor: 'divider',
          cursor: 'pointer',
          '&:hover': { bgcolor: 'action.hover' },
          bgcolor: 'background.paper',
        }}
        onClick={() => toggleExpansion(node.id)}
      >
        <Box sx={{ mr: 1, display: 'flex', alignItems: 'center' }}>
          {node.expanded ? (
            <KeyboardArrowDown
              fontSize="small"
              sx={{ color: 'text.secondary', fontSize: '1rem' }}
            />
          ) : (
            <KeyboardArrowRight
              fontSize="small"
              sx={{ color: 'text.secondary', fontSize: '1rem' }}
            />
          )}
        </Box>
        <Tooltip title={node.group.reason} placement="right" enterDelay={500}>
          <Typography
            variant="body2"
            sx={{
              fontFamily: 'monospace',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              flexGrow: 1,
              maxWidth: 'calc(100% - 80px)', // Reserve space for count
            }}
          >
            {node.group.reason}
          </Typography>
        </Tooltip>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ ml: 1, flexShrink: 0 }}
        >
          ({node.group.verdicts.length})
        </Typography>
      </Box>
    );
  }

  if (node.type === 'verdict') {
    return (
      <VerdictNode
        verdict={node.verdict}
        currentFailureReason={node.parentGroup}
      />
    );
  }

  return null;
}

function getStatusStyle(status: string) {
  if (status === TestVerdict_Status[TestVerdict_Status.FAILED]) {
    return { color: 'error.main' };
  }
  if (status === TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED]) {
    return { color: 'warning.main' };
  }
  if (status === TestVerdict_Status[TestVerdict_Status.FLAKY]) {
    return { color: 'warning.light' };
  }
  return { color: 'text.primary' };
}
