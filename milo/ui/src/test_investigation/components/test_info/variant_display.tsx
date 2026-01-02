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

import { Box, Typography } from '@mui/material';
import { useMemo, useState, useRef, useLayoutEffect, Fragment } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  SummaryLineItem,
  SummaryLineDivider,
} from '@/common/components/page_summary_line';

interface VariantDisplayProps {
  variantDef: { readonly [key: string]: string } | undefined;
  responsive?: boolean;
}

export function VariantDisplay({
  variantDef,
  responsive = false,
}: VariantDisplayProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const measureItemsRef = useRef<(HTMLDivElement | null)[]>([]);
  const [visibleCount, setVisibleCount] = useState<number | null>(null);

  const entries = useMemo(() => Object.entries(variantDef || {}), [variantDef]);

  useLayoutEffect(() => {
    if (!responsive || !containerRef.current) return;

    const calculateVisible = () => {
      const container = containerRef.current;
      if (!container) return;

      const containerWidth = container.getBoundingClientRect().width;
      let currentWidth = 0;
      let count = 0;
      // Reserve space for the "+N more" label and small rounding buffer.
      const MORE_LABEL_ESTIMATE = 100;
      const BUFFER = 2;

      for (let i = 0; i < entries.length; i++) {
        const item = measureItemsRef.current[i];
        if (!item) continue;

        const itemWidth = item.getBoundingClientRect().width;

        if (i === entries.length - 1) {
          // Absolute last item. No need to worry about "+More" label.
          if (currentWidth + itemWidth <= containerWidth) {
            count++;
          }
          break;
        } else {
          // Not the last item, check if it fits PLUS the "more" label.
          if (
            currentWidth + itemWidth + MORE_LABEL_ESTIMATE <=
            containerWidth - BUFFER
          ) {
            currentWidth += itemWidth;
            count++;
          } else {
            break;
          }
        }
      }
      setVisibleCount(count);
    };

    const observer = new ResizeObserver(() => calculateVisible());
    observer.observe(containerRef.current);
    calculateVisible();

    return () => observer.disconnect();
  }, [entries, responsive]);

  if (!variantDef || entries.length === 0) {
    return null;
  }

  if (!responsive) {
    return (
      <>
        {entries.map(([key, value], i) => (
          <Fragment key={key}>
            {i > 0 && <SummaryLineDivider />}
            <SummaryLineItem label={key}>{value}</SummaryLineItem>
          </Fragment>
        ))}
      </>
    );
  }

  const hiddenEntries =
    visibleCount !== null ? entries.slice(visibleCount) : [];

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        flexGrow: 1,
        minWidth: 0,
        position: 'relative',
        height: '100%',
        overflow: 'hidden',
      }}
    >
      {/* Measuring container (hidden) */}
      <Box
        sx={{
          position: 'absolute',
          visibility: 'hidden',
          pointerEvents: 'none',
          display: 'flex',
          whiteSpace: 'nowrap',
          height: 0,
          overflow: 'hidden',
        }}
      >
        {entries.map(([key, value], i) => (
          <Box
            key={key}
            ref={(el) => {
              measureItemsRef.current[i] = el as HTMLDivElement | null;
            }}
            sx={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}
          >
            {i > 0 && <SummaryLineDivider />}
            <SummaryLineItem label={key}>{value}</SummaryLineItem>
          </Box>
        ))}
      </Box>

      {/* Visible container */}
      <Box
        ref={containerRef}
        sx={{
          display: 'flex',
          alignItems: 'center',
          overflow: 'hidden',
          width: '100%',
          visibility: visibleCount === null ? 'hidden' : 'visible',
        }}
      >
        {entries.map(([key, value], i) => (
          <Box
            key={key}
            sx={{
              display: i < (visibleCount ?? 0) ? 'flex' : 'none',
              alignItems: 'center',
              flexShrink: 0,
            }}
          >
            {i > 0 && <SummaryLineDivider />}
            <SummaryLineItem label={key}>{value}</SummaryLineItem>
          </Box>
        ))}
        {visibleCount !== null && visibleCount < entries.length && (
          <HtmlTooltip
            title={
              <Box
                sx={{
                  p: 1.5,
                  display: 'grid',
                  gridTemplateColumns: 'auto 1fr',
                  columnGap: 2,
                  rowGap: 0.5,
                  alignItems: 'center',
                }}
              >
                {hiddenEntries.map(([key, value]) => (
                  <Fragment key={key}>
                    <Typography
                      variant="body2"
                      sx={{ color: 'text.secondary', fontWeight: 500 }}
                    >
                      {key}:
                    </Typography>
                    <Typography variant="body2">{value}</Typography>
                  </Fragment>
                ))}
              </Box>
            }
          >
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              {(visibleCount ?? 0) > 0 && <SummaryLineDivider />}
              <Typography
                variant="body2"
                sx={{
                  cursor: 'help',
                  ml: 0.25,
                  pl: 0.5,
                  pr: 0.5,
                  borderRadius: '4px',
                  color: 'link.main',
                  backgroundColor: 'action.hover',
                  '&:hover': {
                    backgroundColor: 'action.selected',
                  },
                  flexShrink: 0,
                  whiteSpace: 'nowrap',
                }}
              >
                +{entries.length - visibleCount} more
              </Typography>
            </Box>
          </HtmlTooltip>
        )}
      </Box>
    </Box>
  );
}
