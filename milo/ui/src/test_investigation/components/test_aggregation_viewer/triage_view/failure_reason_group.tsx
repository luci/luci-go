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

import { ArrowRight, ExpandMore } from '@mui/icons-material';
import { Box, Collapse, IconButton, Typography } from '@mui/material';

import { TriageGroup, useTriageViewContext } from './context/context';
import { VerdictNode } from './verdict_node';

export interface FailureReasonGroupProps {
  group: TriageGroup;
}

export function FailureReasonGroup({ group }: FailureReasonGroupProps) {
  const { expandedIds, toggleExpansion } = useTriageViewContext();
  const isExpanded = expandedIds.has(group.id);

  return (
    <Box>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          cursor: 'pointer',
          py: 0.5,
          pl: 2,
          '&:hover': { bgcolor: 'action.hover' },
        }}
        onClick={() => toggleExpansion(group.id)}
      >
        <IconButton size="small" sx={{ mr: 1, p: 0.5 }}>
          {isExpanded ? (
            <ExpandMore fontSize="small" />
          ) : (
            <ArrowRight fontSize="small" />
          )}
        </IconButton>
        <Typography
          variant="body2"
          sx={{
            fontFamily: 'monospace',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            flexGrow: 1,
            fontWeight: 'bold',
          }}
          title={group.id}
        >
          {group.id}
        </Typography>
        <Typography
          variant="caption"
          sx={{ ml: 2, mr: 2, color: 'text.secondary' }}
        >
          {group.verdicts.length}
        </Typography>
      </Box>
      <Collapse in={isExpanded} timeout="auto" unmountOnExit>
        <Box>
          {group.verdicts.map((v) => (
            <VerdictNode
              key={v.testId + (v.testIdStructured?.moduleVariantHash || '')}
              verdict={v}
            />
          ))}
        </Box>
      </Collapse>
    </Box>
  );
}
