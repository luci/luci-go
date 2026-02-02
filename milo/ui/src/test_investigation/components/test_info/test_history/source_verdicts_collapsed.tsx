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
import { Button, ButtonGroup, Link, Typography } from '@mui/material';

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { useProject, useTestVariant } from '@/test_investigation/context';

import { useTestVariantBranch } from '../context';
interface SourceVerdictsCollapsedProps {
  segment: Segment;
  expandSegment: () => void;
}

export function SourceVerdictsCollapsed({
  segment,
  expandSegment,
}: SourceVerdictsCollapsedProps) {
  const testVariant = useTestVariant();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();

  const testId = testVariant.testId;
  const variantHash = testVariant.variantHash;
  const refHash = testVariantBranch?.refHash;
  const blamelistBaseUrl = refHash
    ? `/ui/labs/p/${project}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/refs/${refHash}/blamelist`
    : undefined;

  const createBlamelistLink = (position: string) => {
    return `${blamelistBaseUrl}?expand=${`CP-${position}`}#CP-${position}`;
  };
  return (
    <ButtonGroup
      sx={{
        display: 'flex',
        alignItems: 'center',
        width: '100%',
        gap: 0,
        pr: '2px',
      }}
    >
      <Button
        variant="contained"
        sx={{
          backgroundColor: 'grey.100',
          display: 'flex',
          flexDirection: 'column',
          borderRadius: 0,
          padding: 0,
          pl: '10px',
          width: '82px',
          height: '72px',
          '&:hover': {
            backgroundColor: 'grey.200',
          },
          alignItems: 'flex-start',
        }}
      >
        <Typography
          variant="subtitle2"
          component={Link}
          sx={{ textDecoration: 'none', fontWeight: 'normal' }}
        >
          {blamelistBaseUrl ? (
            <Link
              href={createBlamelistLink(segment.endPosition)}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
            >
              {Number(segment.endPosition)}
            </Link>
          ) : (
            Number(segment.endPosition)
          )}
        </Typography>
        {segment.endHour && (
          <>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontStyle: 'italic',
              }}
            >
              {new Date(segment.endHour).toLocaleDateString()}
            </Typography>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontStyle: 'italic',
              }}
            >
              {new Date(segment.endHour).toLocaleTimeString()}
            </Typography>
          </>
        )}
      </Button>
      <Button
        variant="contained"
        sx={{
          backgroundColor: 'grey.100',
          display: 'flex',
          flexDirection: 'column',
          borderRadius: 0,
          padding: 0,
          width: '28px',
          height: '72px',
          alignItems: 'center',
          '&:hover': {
            backgroundColor: 'grey.200',
            boxShadow: 'none',
          },
        }}
        onClick={expandSegment}
      >
        <Typography
          variant="subtitle2"
          sx={{
            color: 'text.secondary',
          }}
        >
          ---
        </Typography>

        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
            fontStyle: 'italic',
            textTransform: 'none',
            fontSize: '10px',
          }}
        >
          {segment?.counts?.totalResults} builds
        </Typography>
      </Button>
      <Button
        variant="contained"
        sx={{
          backgroundColor: 'grey.100',
          display: 'flex',
          flexDirection: 'column',
          borderRadius: 0,
          padding: 0,
          pl: '10px',
          width: '82px',
          height: '72px',
          '&:hover': {
            backgroundColor: 'grey.200',
          },
          alignItems: 'flex-start',
        }}
      >
        <Typography
          variant="subtitle2"
          component={Link}
          sx={{ textDecoration: 'none', fontWeight: 'normal' }}
        >
          {blamelistBaseUrl ? (
            <Link
              href={createBlamelistLink(segment.startPosition)}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
            >
              {Number(segment.startPosition)}
            </Link>
          ) : (
            Number(segment.startPosition)
          )}
        </Typography>
        {segment.startHour && (
          <>
            <Typography
              variant="caption"
              sx={{ color: 'text.secondary', fontStyle: 'italic' }}
            >
              {new Date(segment.startHour).toLocaleDateString()}
            </Typography>
            <Typography
              variant="caption"
              sx={{ color: 'text.secondary', fontStyle: 'italic' }}
            >
              {new Date(segment.startHour).toLocaleTimeString()}
            </Typography>
          </>
        )}
      </Button>
    </ButtonGroup>
  );
}
