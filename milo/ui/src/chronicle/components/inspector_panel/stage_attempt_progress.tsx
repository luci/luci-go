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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import UnfoldLessIcon from '@mui/icons-material/UnfoldLess';
import UnfoldMoreIcon from '@mui/icons-material/UnfoldMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';

import { renderTimestamp } from '@/chronicle/utils/time_utils';
import { Stage_Attempt_Progress } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { AnyDetails } from './any_details';
import { DetailRow } from './detail_row';
import { RenderMode } from './types';

export interface StageAttemptProgressProps {
  progress: readonly Stage_Attempt_Progress[];
  valueDataMap: Map<string, ValueData>;
  renderMode?: RenderMode;
}

export function StageAttemptProgress({
  progress,
  valueDataMap,
  renderMode,
}: StageAttemptProgressProps) {
  // Expand the most recent (last) progress update by default.
  const [openIndices, setOpenIndices] = useState<Set<number>>(
    () => new Set(progress && progress.length > 0 ? [progress.length - 1] : []),
  );

  useEffect(() => {
    setOpenIndices(
      new Set(progress && progress.length > 0 ? [progress.length - 1] : []),
    );
  }, [progress]);

  if (!progress || progress.length === 0) {
    return null;
  }

  const handleExpandAll = () => {
    setOpenIndices(new Set(progress.map((_, i) => i)));
  };

  const handleCollapseAll = () => {
    setOpenIndices(new Set());
  };

  const allExpanded = openIndices.size === progress.length;
  const allCollapsed = openIndices.size === 0;

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', mt: 0.5 }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'flex-end',
          alignItems: 'center',
          gap: 0.5,
          mb: 0.5,
        }}
      >
        <Tooltip title="Expand all">
          <span>
            <IconButton
              size="small"
              onClick={handleExpandAll}
              disabled={allExpanded}
              aria-label="expand all"
              sx={{ p: 0.25 }}
            >
              <UnfoldMoreIcon fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Collapse all">
          <span>
            <IconButton
              size="small"
              onClick={handleCollapseAll}
              disabled={allCollapsed}
              aria-label="collapse all"
              sx={{ p: 0.25 }}
            >
              <UnfoldLessIcon fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
      </Box>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 1,
          p: 1,
          border: '1px solid var(--divider-color)',
          borderRadius: 1,
          bgcolor: 'var(--block-background-color)',
        }}
      >
        {progress.map((prog, pIndex) => (
          <Accordion
            key={pIndex}
            expanded={openIndices.has(pIndex)}
            onChange={(_, isExpanded) => {
              setOpenIndices((prev) => {
                const next = new Set(prev);
                if (isExpanded) {
                  next.add(pIndex);
                } else {
                  next.delete(pIndex);
                }
                return next;
              });
            }}
            disableGutters
            variant="outlined"
          >
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 600,
                  display: '-webkit-box',
                  WebkitLineClamp: 1,
                  WebkitBoxOrient: 'vertical',
                  overflow: 'hidden',
                  wordBreak: 'break-all',
                  flex: 1,
                  mr: 1,
                }}
              >
                {`Update ${pIndex + 1}${prog.message ? `: ${prog.message}` : ''}`}
              </Typography>
              {prog.version?.ts && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ flexShrink: 0 }}
                >
                  {renderTimestamp(prog.version.ts)}
                </Typography>
              )}
            </AccordionSummary>
            <AccordionDetails
              sx={{
                p: 1,
                display: 'flex',
                flexDirection: 'column',
                gap: 0.5,
              }}
            >
              {prog.version?.ts && (
                <DetailRow
                  label="Timestamp"
                  value={renderTimestamp(prog.version.ts)}
                />
              )}
              {prog.message && (
                <DetailRow label="Message" value={prog.message} />
              )}
              {prog.details?.map((detail, dIndex) => {
                const valueData = detail.digest
                  ? valueDataMap.get(detail.digest)
                  : undefined;
                return (
                  <AnyDetails
                    key={dIndex}
                    typeUrl={detail.typeUrl}
                    omitReason={detail.omitReason}
                    valueData={valueData}
                    json={valueData?.json?.value}
                    label="Details"
                    renderMode={renderMode}
                    // Progress items are enclosed in collapsible accordions, so display full content without inner scrolling.
                    maxHeight="none"
                  />
                );
              })}
            </AccordionDetails>
          </Accordion>
        ))}
      </Box>
    </Box>
  );
}
