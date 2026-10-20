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

import { Box, Typography } from '@mui/material';
import { useState } from 'react';

import { colors } from '@/fleet/theme/colors';

import { HealthTooltip } from './health_tooltip';
import {
  getAnnotationTimestampMs,
  TrendlineAnnotation,
} from './trendline_annotations';
import { formatTooltipDate } from './trends_chart_data';

/**
 * Shape props provided by Recharts ReferenceLine when rendering a custom component.
 */
export interface ReferenceLineShapeProps {
  readonly x1?: number;
  readonly y1?: number;
  readonly x2?: number;
  readonly y2?: number;
}

export interface AnnotationMarkerProps extends ReferenceLineShapeProps {
  readonly annotation: TrendlineAnnotation;
}

/**
 * Renders a vertical event marker line on historical trendline charts, complete
 * with an interactive information badge and descriptive tooltip.
 */
export const AnnotationMarker = ({
  x1,
  y1,
  x2,
  y2,
  annotation,
}: AnnotationMarkerProps) => {
  const [isHovered, setIsHovered] = useState(false);
  const [isFocused, setIsFocused] = useState(false);

  if (
    x1 === undefined ||
    y1 === undefined ||
    x2 === undefined ||
    y2 === undefined ||
    !Number.isFinite(x1) ||
    !Number.isFinite(x2) ||
    !Number.isFinite(y1) ||
    !Number.isFinite(y2)
  ) {
    return null;
  }

  const topY = Math.min(y1, y2);
  const bottomY = Math.max(y1, y2);
  const isActive = isHovered || isFocused;
  const markerColor = isActive ? colors.grey[500] : colors.grey[400];
  const badgeRadius = 9;
  const badgeCenterY = topY + 8;
  const ts = getAnnotationTimestampMs(annotation);

  return (
    <HealthTooltip
      variant="standard"
      title={
        <Box sx={{ maxWidth: 300, p: 0.5 }}>
          <Typography
            variant="subtitle2"
            sx={{ fontWeight: 'bold', color: 'text.primary', mb: 0.25 }}
          >
            {annotation.title}
          </Typography>
          {ts !== undefined && (
            <Typography
              variant="caption"
              sx={{ color: 'text.secondary', display: 'block', mb: 0.75 }}
            >
              {formatTooltipDate(ts)}
            </Typography>
          )}
          <Typography
            variant="body2"
            sx={{ color: 'text.primary', lineHeight: 1.4 }}
          >
            {annotation.description}
          </Typography>
        </Box>
      }
      placement="top"
    >
      <g
        data-testid={`trendline-annotation-${ts}`}
        role="img"
        aria-label={`${annotation.title}: ${annotation.description}`}
        tabIndex={0}
        style={{ pointerEvents: 'all', cursor: 'pointer', outline: 'none' }}
        onMouseEnter={() => setIsHovered(true)}
        onMouseLeave={() => setIsHovered(false)}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
      >
        {/* Visible vertical dashed reference line */}
        <line
          data-testid="trendline-annotation-line"
          x1={x1}
          y1={bottomY}
          x2={x2}
          y2={topY}
          stroke={markerColor}
          strokeWidth={isActive ? 2 : 1.5}
          strokeDasharray="4 4"
        />
        {/* Invisible wider hit area along the line for easy mouse interaction */}
        <line
          data-testid="trendline-annotation-hit-area"
          x1={x1}
          y1={bottomY}
          x2={x2}
          y2={topY}
          stroke="transparent"
          strokeWidth={16}
        />
        {/* Keyboard focus ring around circular badge */}
        {isFocused && (
          <circle
            cx={x2}
            cy={badgeCenterY}
            r={badgeRadius + 3}
            fill="none"
            stroke={colors.grey[500]}
            strokeWidth={1.5}
          />
        )}
        {/* Badge circle at the top of the line */}
        <circle
          data-testid="trendline-annotation-badge"
          cx={x2}
          cy={badgeCenterY}
          r={badgeRadius}
          fill={isActive ? markerColor : colors.white}
          stroke={markerColor}
          strokeWidth={1.5}
        />
        {/* "i" icon letter inside the badge */}
        <text
          x={x2}
          y={badgeCenterY}
          textAnchor="middle"
          dominantBaseline="central"
          fill={isActive ? colors.white : colors.grey[600]}
          fontSize={11}
          fontWeight="bold"
          fontFamily="sans-serif"
          style={{ userSelect: 'none' }}
        >
          i
        </text>
      </g>
    </HealthTooltip>
  );
};
