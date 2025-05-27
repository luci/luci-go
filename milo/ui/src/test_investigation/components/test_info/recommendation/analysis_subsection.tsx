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

import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { Box, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import React, { useMemo, createElement } from 'react';

import { getStatusStyle } from '@/common/styles/status_styles';
import { useInvocation, useTestVariant } from '@/test_investigation/context';

import { useTestVariantBranch } from '../context/context';

import { AnalysisItemContent, generateAnalysisPoints } from './analysis_utils';

interface AnalysisItemProps {
  item: AnalysisItemContent;
}

function AnalysisItem({ item }: AnalysisItemProps) {
  let iconElement: React.ReactElement | null = null;
  if (item.status) {
    const style = getStatusStyle(item.status);
    if (style.icon) {
      iconElement = createElement(style.icon, {
        sx: {
          fontSize: 18,
          color: style.iconColor || style.textColor,
        },
      });
    } else {
      iconElement = (
        <InfoOutlinedIcon
          sx={{ fontSize: 18, color: style.textColor || 'action' }}
        />
      );
    }
  } else {
    iconElement = <InfoOutlinedIcon sx={{ fontSize: 18 }} color="action" />;
  }

  return (
    <Box
      sx={{ display: 'flex', alignItems: 'flex-start', width: '100%', mb: 1 }}
    >
      {iconElement && <Box sx={{ mr: 1, mt: '3px' }}>{iconElement}</Box>}
      <Box sx={{ flexGrow: 1 }}>
        <Typography
          variant="body2"
          component="div"
          sx={{ '& > p': { margin: 0 }, whiteSpace: 'pre-line' }}
        >
          {item.text}
        </Typography>
      </Box>
    </Box>
  );
}

interface AnalysisSubsectionProps {
  currentTimeForAgoDt: DateTime;
}

export function AnalysisSubsection({
  currentTimeForAgoDt,
}: AnalysisSubsectionProps) {
  const invocation = useInvocation();
  const testVariant = useTestVariant();
  const testVariantBranch = useTestVariantBranch();

  const analysisItems = useMemo(() => {
    const rawSegments = testVariantBranch?.segments;

    return generateAnalysisPoints(
      currentTimeForAgoDt,
      rawSegments,
      invocation,
      testVariant,
    );
  }, [invocation, testVariant, testVariantBranch, currentTimeForAgoDt]);

  return (
    <Box sx={{ flex: 1 }}>
      <Typography
        variant="subtitle1"
        component="div"
        gutterBottom
        sx={{ fontWeight: 'medium' }}
      >
        Analysis
      </Typography>

      {analysisItems.length > 0 ? (
        analysisItems.map((itemContent, i) => (
          <AnalysisItem key={i} item={itemContent} />
        ))
      ) : (
        <Typography variant="body2" color="text.disabled">
          There are no analysis findings.
        </Typography>
      )}
    </Box>
  );
}
