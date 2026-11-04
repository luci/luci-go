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

import { Box, Link, Typography } from '@mui/material';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { surfaceCardClippedSx } from '@/fleet/theme/recipes';
import { borders, fontFamilies } from '@/fleet/theme/tokens';

import {
  formatProductCategory,
  formatUnitCost,
  getPlmStatusTone,
  resolveProductType,
} from './catalog_details_helpers';
import { CatalogHeroPhotoStage } from './catalog_hero_photo_stage';
import { UnifiedProductCatalogEntry } from './types';

export interface CatalogDetailsHeroProps {
  readonly entry: UnifiedProductCatalogEntry;
  readonly stagedPhotos: readonly string[];
  readonly onOpenPhotoEditor: () => void;
  readonly onPhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

const buildHeroSummaryMetrics = (
  entry: UnifiedProductCatalogEntry,
  isGce: boolean,
  formattedCost: string,
) => {
  const costItem =
    formattedCost !== '—'
      ? [{ label: 'Estimated Unit Cost', value: formattedCost, mono: false }]
      : [];
  if (isGce) {
    return [
      { label: 'CPU Architecture', value: entry.cpuType || '—', mono: false },
      {
        label: 'VM Compute Profile',
        value: entry.cpuNumPerVm
          ? `${entry.cpuNumPerVm} vCPU • ${entry.memoryGbPerVm ?? 0} GB`
          : '—',
        mono: false,
      },
      ...costItem,
      {
        label: 'Fulfillment Model',
        value: 'Cloud Virtual Instance',
        mono: false,
      },
    ];
  }
  return [
    {
      label: 'Google Part Number (GPN)',
      value: entry.gpn || '—',
      mono: Boolean(entry.gpn),
    },
    { label: 'Resource Type', value: entry.resourceType || '—', mono: false },
    ...costItem,
    {
      label: 'Devices per Rack',
      value:
        entry.numberOfDevicesPerRack !== undefined &&
        entry.numberOfDevicesPerRack > 0
          ? `${entry.numberOfDevicesPerRack} units / rack`
          : '—',
      mono: false,
    },
  ];
};

export const CatalogDetailsHero = ({
  entry,
  stagedPhotos,
  onOpenPhotoEditor,
  onPhotoDimensionsEvaluated,
}: CatalogDetailsHeroProps) => {
  const resolvedProductType = resolveProductType(
    entry.productType,
    entry.productCatalogId,
  );
  const isGce = resolvedProductType === 'gce';
  const summaryMetrics = buildHeroSummaryMetrics(
    entry,
    isGce,
    formatUnitCost(entry.unitCost),
  );
  const totalSummaryCols = summaryMetrics.length + (isGce ? 0 : 1);

  return (
    <Box
      data-testid="catalog-details-hero"
      sx={{
        ...surfaceCardClippedSx,
        mb: 3,
        display: 'grid',
        gridTemplateColumns: { xs: '1fr', md: '360px 1fr' },
        alignItems: 'stretch',
      }}
    >
      <CatalogHeroPhotoStage
        entry={entry}
        resolvedProductType={resolvedProductType}
        isGce={isGce}
        stagedPhotos={stagedPhotos}
        onOpenPhotoEditor={onOpenPhotoEditor}
        onPhotoDimensionsEvaluated={onPhotoDimensionsEvaluated}
      />

      <Box
        sx={{
          p: { xs: 2, md: 2.5 },
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'flex-start',
          gap: 2,
          minWidth: 0,
        }}
      >
        <Box>
          <Box
            sx={{
              display: 'flex',
              flexWrap: 'wrap',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 1,
              mb: 0.5,
            }}
          >
            <Typography
              variant="h6"
              sx={{ color: 'text.primary', fontWeight: 700 }}
            >
              {entry.productName ||
                entry.descriptiveName ||
                entry.productCatalogId}
            </Typography>
            <Box
              sx={{
                display: 'flex',
                flexWrap: 'wrap',
                alignItems: 'center',
                gap: 1,
              }}
            >
              <StatusPill tone="info" dot={false} size="compact">
                {formatProductCategory(
                  entry.productType,
                  entry.productCatalogId,
                )}
              </StatusPill>
              {entry.fleetPlmStatus && (
                <StatusPill
                  tone={getPlmStatusTone(entry.fleetPlmStatus)}
                  size="compact"
                  dot={false}
                >
                  {entry.fleetPlmStatus}
                </StatusPill>
              )}
            </Box>
          </Box>
          <Typography
            variant="caption"
            sx={{
              color: 'text.secondary',
              display: 'block',
              fontFamily: fontFamilies.mono,
            }}
          >
            ID: {entry.productCatalogId}
            {entry.descriptiveName &&
            entry.descriptiveName !== entry.productName
              ? ` · ${entry.descriptiveName}`
              : ''}
          </Typography>
        </Box>

        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: {
              xs: 'repeat(2, minmax(0, 1fr))',
              lg: `repeat(${Math.max(totalSummaryCols, 3)}, minmax(0, 1fr))`,
            },
            gap: 2,
            pt: 1.75,
            borderTop: borders.divider,
          }}
        >
          {summaryMetrics.map((metric) => (
            <Box key={metric.label} sx={{ minWidth: 0 }}>
              <Typography
                variant="caption"
                sx={{ display: 'block', color: 'text.secondary', mb: 0.25 }}
              >
                {metric.label}
              </Typography>
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 500,
                  color: 'text.primary',
                  ...(metric.mono ? { fontFamily: fontFamilies.mono } : {}),
                }}
              >
                {metric.value}
              </Typography>
            </Box>
          ))}
          {!isGce &&
            (() => {
              const r11nItems = (entry.r11n ?? [])
                .map((val) => val.trim())
                .filter(Boolean);
              return (
                <Box sx={{ minWidth: 0 }}>
                  <Typography
                    variant="caption"
                    sx={{ display: 'block', color: 'text.secondary', mb: 0.25 }}
                  >
                    R11N Build Guides
                  </Typography>
                  <Typography
                    variant="body2"
                    component="div"
                    sx={{ fontWeight: 500 }}
                  >
                    {r11nItems.length > 0
                      ? r11nItems.map((item, idx) => (
                          <span key={`${item}-${idx}`}>
                            {idx > 0 && ', '}
                            <Link
                              href={`https://goto.google.com/ngp-npi/r11n/${encodeURIComponent(item.toLowerCase())}`}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              {item}
                            </Link>
                          </span>
                        ))
                      : '—'}
                  </Typography>
                </Box>
              );
            })()}
        </Box>
      </Box>
    </Box>
  );
};
