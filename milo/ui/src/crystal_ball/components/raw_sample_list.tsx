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

import { ExpandMore as ExpandMoreIcon } from '@mui/icons-material';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Card,
  Link,
  Typography,
} from '@mui/material';
import React from 'react';

import {
  AB_BASE_URL,
  ARC_REGRESSION_DASHBOARD_URL,
  ATP_BASE_URL,
  Column,
  COMMON_MESSAGES,
} from '@/crystal_ball/constants';
import type { RawSampleRow } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

const formatMtvTimestamp = (ts: unknown) => {
  if (typeof ts !== 'string') return '';
  const date = new Date(ts);
  const formatter = new Intl.DateTimeFormat('sv-SE', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: 'America/Los_Angeles',
  });
  return `${formatter.format(date)} MTV`;
};

interface RawSampleItemProps {
  row: RawSampleRow;
  expanded: boolean;
  onChange: (event: React.SyntheticEvent, expanded: boolean) => void;
}

function RawSampleItem({ row, expanded, onChange }: RawSampleItemProps) {
  // Safe URL construction using URL and URLSearchParams
  const atpTestName = String(row.values?.[Column.ATP_TEST_NAME] ?? '');
  const buildBranch = String(row.values?.[Column.BUILD_BRANCH] ?? '');
  const buildTarget = String(row.values?.[Column.BUILD_TARGET] ?? '');
  const buildCreationTs = row.values?.['build_creation_timestamp'];
  const testFinishedTs = row.values?.[Column.INVOCATION_COMPLETE_TIMESTAMP];

  const atpUrl = new URL(
    `${ATP_BASE_URL}/tests/${encodeURIComponent(atpTestName)}`,
  );
  atpUrl.searchParams.set('tabId', 'test_run');
  atpUrl.searchParams.set('branchName', buildBranch);
  atpUrl.searchParams.set('offset', '0');
  atpUrl.searchParams.set('buildTarget', buildTarget);

  const arcUrl = new URL(ARC_REGRESSION_DASHBOARD_URL);
  arcUrl.searchParams.append('f', `atp_test_name:eq:${atpTestName}`);
  arcUrl.searchParams.append('f', `build_branch:in:${buildBranch}`);
  arcUrl.searchParams.append('f', `build_target:in:${buildTarget}`);
  arcUrl.searchParams.append(
    'f',
    `metric_key:eq:${String(row.values?.[Column.METRIC_KEY] ?? '')}`,
  );
  arcUrl.searchParams.append(
    'f',
    `build_id:gte:${String(row.values?.[Column.BUILD_ID] ?? '')}`,
  );
  arcUrl.searchParams.append(
    'f',
    `test_name:ct:${String(row.values?.[Column.TEST_NAME] ?? '')}`,
  );

  // Option A: Format Value
  const rawVal = row.values?.[Column.VALUE];
  const numVal = Number(rawVal);
  const formattedValue =
    !isNaN(numVal) && rawVal !== '' ? numVal.toLocaleString() : (rawVal ?? '');

  return (
    <Card
      variant="outlined"
      sx={{
        width: '100%',
        minWidth: 0,
        borderRadius: 1,
        overflow: 'hidden',
        '&:hover': {
          boxShadow: (theme) => theme.shadows[1],
        },
      }}
    >
      {/* Value Header */}
      <Box
        sx={{
          px: 2,
          py: 1,
          bgcolor: 'action.hover',
          borderBottom: '1px solid',
          borderColor: 'divider',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography
            variant="subtitle2"
            sx={{
              fontFamily: 'monospace',
              fontWeight: (theme) => theme.typography.fontWeightBold,
              color: 'text.primary',
            }}
          >
            {formattedValue}
          </Typography>
        </Box>

        {/* Action Links (Reordered: ATI, ATP, ARC, Perfetto) */}
        <Box sx={{ display: 'flex', gap: 1.5 }}>
          {row.values?.[Column.INVOCATION_URL] && (
            <Link
              href={String(row.values[Column.INVOCATION_URL])}
              target="_blank"
              rel="noopener noreferrer"
              variant="caption"
              sx={{
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textDecoration: 'none',
              }}
            >
              ATI
            </Link>
          )}
          <Link
            href={atpUrl.toString()}
            target="_blank"
            rel="noopener noreferrer"
            variant="caption"
            sx={{
              fontWeight: (theme) => theme.typography.fontWeightBold,
              textDecoration: 'none',
            }}
          >
            ATP
          </Link>
          <Link
            href={arcUrl.toString()}
            target="_blank"
            rel="noopener noreferrer"
            variant="caption"
            sx={{
              fontWeight: (theme) => theme.typography.fontWeightBold,
              textDecoration: 'none',
            }}
          >
            ARC
          </Link>
          {row.values?.[Column.PERFETTO_ARTIFACT_URL] && (
            <Link
              href={String(row.values[Column.PERFETTO_ARTIFACT_URL])}
              target="_blank"
              rel="noopener noreferrer"
              variant="caption"
              sx={{
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textDecoration: 'none',
              }}
            >
              Perfetto
            </Link>
          )}
        </Box>
      </Box>

      {/* Test & Metric Info */}
      {/* Body: High-Density Data Grid */}
      <Box sx={{ p: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
        {/* Meta Info: Timings, ID, Branch & Target */}
        {/* Grid for ID and Timestamps */}
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: 'max-content 1fr',
            gap: 0.5,
            alignItems: 'baseline',
          }}
        >
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 'medium' }}
          >
            Build
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Link
              href={`${AB_BASE_URL}/${row.values?.[Column.BUILD_ID]}`}
              target="_blank"
              rel="noopener noreferrer"
              variant="caption"
              sx={{
                fontFamily: 'monospace',
                textDecoration: 'none',
                fontWeight: 'bold',
                color: 'primary.main',
              }}
            >
              {String(row.values?.[Column.BUILD_ID] ?? '')}
            </Link>
            <Typography variant="caption" color="text.secondary">
              |
            </Typography>
            <Typography
              variant="caption"
              sx={{
                color: 'text.primary',
                fontFamily: 'monospace',
                wordBreak: 'break-all',
              }}
            >
              {buildBranch}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              |
            </Typography>
            <Typography
              variant="caption"
              color="text.primary"
              sx={{ wordBreak: 'break-all', fontFamily: 'monospace' }}
              title={buildTarget}
            >
              {buildTarget}
            </Typography>
          </Box>

          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 'medium' }}
          >
            Build Created
          </Typography>
          <Typography
            variant="caption"
            sx={{ fontFamily: 'monospace', color: 'text.primary' }}
          >
            {formatMtvTimestamp(buildCreationTs)}
          </Typography>

          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 'medium' }}
          >
            Test Finished
          </Typography>
          <Typography
            variant="caption"
            sx={{ fontFamily: 'monospace', color: 'text.primary' }}
          >
            {formatMtvTimestamp(testFinishedTs)}
          </Typography>

          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 'medium' }}
          >
            Atp Test Name
          </Typography>
          <Typography
            variant="caption"
            sx={{
              fontFamily: 'monospace',
              color: 'text.primary',
              wordBreak: 'break-all',
            }}
          >
            {atpTestName}
          </Typography>

          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 'medium' }}
          >
            Test Name
          </Typography>
          <Typography
            variant="caption"
            sx={{
              fontFamily: 'monospace',
              color: 'text.primary',
              wordBreak: 'break-all',
            }}
          >
            {String(row.values?.[Column.TEST_NAME] ?? '')}
          </Typography>
        </Box>
      </Box>

      {/* All Details */}
      <Accordion
        disableGutters
        elevation={0}
        expanded={expanded}
        onChange={onChange}
        sx={{
          borderTop: '1px solid',
          borderColor: 'divider',
          '&:before': { display: 'none' },
        }}
      >
        <AccordionSummary
          expandIcon={<ExpandMoreIcon fontSize="small" />}
          sx={{
            minHeight: 28,
            '& .MuiAccordionSummary-content': { my: 0.25 },
          }}
        >
          <Typography
            variant="caption"
            sx={{
              fontWeight: (theme) => theme.typography.fontWeightBold,
              color: 'text.secondary',
              textTransform: 'uppercase',
            }}
          >
            {COMMON_MESSAGES.ALL_DETAILS}
          </Typography>
        </AccordionSummary>
        <AccordionDetails
          sx={{
            px: 2,
            pb: 2,
            pt: 0,
            display: 'grid',
            gridTemplateColumns: 'max-content 1fr',
            gap: 0.5,
            alignItems: 'baseline',
            bgcolor: 'action.hover',
          }}
        >
          {Object.entries(row.values ?? {})
            .filter(
              ([key, val]) =>
                val !== null &&
                val !== undefined &&
                val !== '' &&
                key !== Column.INVOCATION_URL &&
                key !== Column.PERFETTO_ARTIFACT_URL,
            )
            .sort(([keyA], [keyB]) => keyA.localeCompare(keyB))
            .map(([key, val]) => {
              const formattedKey =
                key
                  .replace(/_/g, ' ')
                  .replace(/\b\w/g, (l) => l.toUpperCase()) + ':';
              return (
                <React.Fragment key={key}>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{
                      fontWeight: 'medium',
                      pr: 1,
                    }}
                  >
                    {formattedKey}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.primary"
                    sx={{ wordBreak: 'break-all', fontFamily: 'monospace' }}
                  >
                    {String(val)}
                  </Typography>
                </React.Fragment>
              );
            })}
        </AccordionDetails>
      </Accordion>
    </Card>
  );
}

/**
 * Props for the RawSampleList component.
 */
export interface RawSampleListProps {
  /** The raw sample rows to display. */
  rows: readonly RawSampleRow[];
  /** The set of expanded row indices. */
  expandedItems: Set<number>;
  /** Callback triggered when a row is toggled. */
  onToggleExpand: (index: number) => void;
}

/**
 * Component to display a list of raw samples.
 */
export function RawSampleList({
  rows,
  expandedItems,
  onToggleExpand,
}: RawSampleListProps) {
  if (rows.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
        {COMMON_MESSAGES.NO_RAW_SAMPLES_FOUND}
      </Typography>
    );
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
      {rows.map((row, rowIndex) => (
        <RawSampleItem
          key={rowIndex}
          row={row}
          expanded={expandedItems.has(rowIndex)}
          onChange={() => onToggleExpand(rowIndex)}
        />
      ))}
    </Box>
  );
}
