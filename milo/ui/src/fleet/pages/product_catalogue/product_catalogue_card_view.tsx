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

import styled from '@emotion/styled';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import CloudOutlinedIcon from '@mui/icons-material/CloudOutlined';
import DevicesIcon from '@mui/icons-material/Devices';
import DnsIcon from '@mui/icons-material/Dns';
import LaptopIcon from '@mui/icons-material/Laptop';
import PhoneAndroidIcon from '@mui/icons-material/PhoneAndroid';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Grid,
  IconButton,
  Skeleton,
  TablePagination,
  Typography,
} from '@mui/material';
import { MRT_PaginationState, MRT_Updater } from 'material-react-table';
import { useState } from 'react';
import { Link } from 'react-router';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { useProductCatalogRevamp } from '@/fleet/features';
import { colors } from '@/fleet/theme/colors';
import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
  StatusTone,
} from '@/fleet/theme/tokens';

import { UnifiedProductCatalogEntry } from './types';

enum PlmStatus {
  GA = 'GA',
  LA = 'LA',
  NPI = 'NPI',
  ACTIVE = 'ACTIVE',
}

enum ProductType {
  ANDROID_TESTBED = 'android-testbed',
  OS_TESTBED = 'os-testbed',
  HARDWARE = 'hardware',
  PERIPHERALS = 'peripherals',
  GCE = 'gce',
}

const getPlmStatusTone = (status?: string): StatusTone => {
  switch (status?.toUpperCase()) {
    case PlmStatus.GA:
    case PlmStatus.ACTIVE:
      return 'success';
    case PlmStatus.LA:
      return 'warning';
    case PlmStatus.NPI:
      return 'info';
    default:
      return 'neutral';
  }
};

const formatUnitCost = (unitCost?: string) => {
  if (!unitCost || unitCost === '0') return null;
  const trimmed = unitCost.trim();
  if (trimmed.startsWith('$')) {
    return trimmed;
  }
  const numeric = Number(trimmed.replace(/,/g, ''));
  if (!Number.isNaN(numeric) && numeric > 0) {
    return `$${numeric.toLocaleString('en-US')}`;
  }
  return `$${trimmed}`;
};

const getProductIcon = (productType?: string) => {
  const prodType = (productType || '').trim().toLowerCase();
  const iconSx = {
    fontSize: 40,
    color: fleetColors.primary,
    flexShrink: 0,
    display: 'block',
  };
  switch (prodType) {
    case ProductType.ANDROID_TESTBED:
      return <PhoneAndroidIcon sx={iconSx} />;
    case ProductType.OS_TESTBED:
      return <DnsIcon sx={iconSx} />;
    case ProductType.GCE:
      return <CloudOutlinedIcon sx={iconSx} />;
    case ProductType.HARDWARE:
      return <LaptopIcon sx={iconSx} />;
    case ProductType.PERIPHERALS:
    default:
      return <DevicesIcon sx={iconSx} />;
  }
};

const DEFAULT_PAGE_SIZE_OPTIONS = [20, 50, 100];

const CardWrapper = styled(Card)`
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  height: 100%;
  border: ${borders.default};
  border-radius: ${radii.md};
  background-color: ${fleetColors.surface};
  box-shadow: ${elevations.card};
  overflow: hidden;
  transition:
    transform 0.2s ease-in-out,
    border-color 0.2s ease-in-out;
  cursor: pointer;
  text-decoration: none;
  color: inherit;

  &:hover {
    transform: translateY(-3px);
    border-color: ${fleetColors.borderHover};
  }
`;

const MediaStage = styled(Box)`
  box-sizing: border-box;
  position: relative;
  height: 184px;
  min-height: 184px;
  width: 100%;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  background: linear-gradient(
    135deg,
    ${fleetColors.surfaceTonal} 0%,
    ${fleetColors.primaryContainer} 100%
  );
  border-bottom: ${borders.divider};
  overflow: hidden;
  padding: 10px 12px;
`;

const ContentSection = styled(CardContent)`
  box-sizing: border-box;
  flex-grow: 1;
  padding: 16px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 12px;
  &:last-child {
    padding-bottom: 16px;
  }
`;

interface ProductCatalogueCardViewProps {
  entries: UnifiedProductCatalogEntry[];
  isLoading: boolean;
  pagination: MRT_PaginationState;
  onPaginationChange: (updater: MRT_Updater<MRT_PaginationState>) => void;
  totalCount: number;
}

const getPathnameWithParams = () => {
  return window.location.href.toString().split(window.location.host)[1];
};

export interface CategoryIllustrationProps {
  readonly productType?: string;
  readonly productCatalogId: string;
}

export const CategoryIllustration = ({
  productType,
  productCatalogId,
}: CategoryIllustrationProps) => {
  return (
    <Box
      data-testid={`catalog-photo-fallback-${productCatalogId}`}
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        my: 'auto',
      }}
    >
      {getProductIcon(productType)}
    </Box>
  );
};

interface CatalogEntryCardProps {
  readonly entry: UnifiedProductCatalogEntry;
}

const CatalogEntryCard = ({ entry }: CatalogEntryCardProps) => {
  const [activePhotoIdx, setActivePhotoIdx] = useState(0);
  const [failedUris, setFailedUris] = useState<Record<string, boolean>>({});
  const formattedCost = formatUnitCost(entry.unitCost);
  const specs: Array<{
    readonly label: string;
    readonly value: string | number;
    readonly mono?: boolean;
  }> = [
    ...(entry.gpn ? [{ label: 'GPN', value: entry.gpn, mono: true }] : []),
    ...(entry.resourceType
      ? [{ label: 'Resource Type', value: entry.resourceType }]
      : []),
    ...(entry.r11n && entry.r11n.length > 0
      ? [{ label: 'R11N', value: entry.r11n.join(', ') }]
      : []),
    ...(formattedCost ? [{ label: 'Unit Cost', value: formattedCost }] : []),
    ...(entry.numberOfDevicesPerRack !== undefined &&
    entry.numberOfDevicesPerRack !== 0
      ? [{ label: 'Per Rack', value: entry.numberOfDevicesPerRack }]
      : []),
    ...(entry.cpuType ? [{ label: 'CPU Type', value: entry.cpuType }] : []),
    ...(entry.cpuNumPerVm !== undefined && entry.cpuNumPerVm !== 0
      ? [{ label: 'CPU / VM', value: entry.cpuNumPerVm }]
      : []),
    ...(entry.memoryGbPerVm !== undefined && entry.memoryGbPerVm !== 0
      ? [{ label: 'Memory / VM', value: `${entry.memoryGbPerVm} GB` }]
      : []),
  ];

  const photos = (entry.photoDataUris ?? []).filter(
    (uri) => Boolean(uri) && !failedUris[uri],
  );
  const safePhotoIdx = photos.length > 0 ? activePhotoIdx % photos.length : 0;
  const currentPhotoUri = photos[safePhotoIdx];
  const hasPhoto = photos.length > 0;

  const handlePrevPhoto = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setActivePhotoIdx((prev) => {
      const idx = prev % photos.length;
      return idx > 0 ? idx - 1 : photos.length - 1;
    });
  };

  const handleNextPhoto = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setActivePhotoIdx((prev) => {
      const idx = prev % photos.length;
      return idx < photos.length - 1 ? idx + 1 : 0;
    });
  };

  return (
    <Box sx={{ position: 'relative', height: '100%' }}>
      <Link
        to={`/ui/fleet/catalog/${entry.productCatalogId}`}
        state={{
          navigatedFromLink: getPathnameWithParams(),
        }}
        style={{
          textDecoration: 'none',
          color: 'inherit',
          display: 'block',
          height: '100%',
        }}
      >
        <CardWrapper data-testid={`catalog-card-${entry.productCatalogId}`}>
          <MediaStage>
            {hasPhoto ? (
              <>
                <Box
                  component="img"
                  className="catalog-photo-img"
                  src={currentPhotoUri}
                  alt={`${entry.productName || entry.productCatalogId} hardware reference`}
                  data-testid={`catalog-photo-img-${entry.productCatalogId}`}
                  loading="lazy"
                  onError={() =>
                    setFailedUris((prev) => ({
                      ...prev,
                      [currentPhotoUri]: true,
                    }))
                  }
                  sx={{
                    position: 'absolute',
                    inset: 0,
                    width: '100%',
                    height: '100%',
                    objectFit: 'cover',
                    objectPosition: 'center',
                    display: 'block',
                    zIndex: 0,
                  }}
                />

                {photos.length > 1 && (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'flex-end',
                      width: '100%',
                      mt: 'auto',
                      zIndex: 2,
                    }}
                  >
                    <Box
                      sx={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        px: 1,
                        height: controlHeights.chipCompact,
                        borderRadius: radii.pill,
                        backgroundColor: colors.grey[900],
                        color: fleetColors.surface,
                        boxShadow: elevations.card,
                      }}
                    >
                      <Typography
                        variant="caption"
                        sx={{
                          fontSize: fontSizes.caption,
                          fontWeight: 500,
                          lineHeight: 1,
                        }}
                      >
                        {`${safePhotoIdx + 1} / ${photos.length}`}
                      </Typography>
                    </Box>
                  </Box>
                )}
              </>
            ) : (
              <CategoryIllustration
                productType={entry.productType}
                productCatalogId={entry.productCatalogId}
              />
            )}
          </MediaStage>

          <ContentSection>
            <Box>
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'flex-start',
                  gap: 1,
                  mb: 0.25,
                }}
              >
                <Typography
                  variant="h6"
                  title={
                    entry.productName ||
                    entry.descriptiveName ||
                    entry.productCatalogId
                  }
                  sx={{
                    color: 'text.primary',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    display: '-webkit-box',
                    WebkitLineClamp: 2,
                    WebkitBoxOrient: 'vertical',
                  }}
                >
                  {entry.productName ||
                    entry.descriptiveName ||
                    entry.productCatalogId}
                </Typography>
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
              <Typography
                variant="caption"
                sx={{
                  color: 'text.secondary',
                  display: 'block',
                  mb:
                    entry.descriptiveName &&
                    entry.descriptiveName !== entry.productName
                      ? 1
                      : 0,
                  fontFamily: fontFamilies.mono,
                }}
              >
                ID: {entry.productCatalogId}
              </Typography>
              {entry.descriptiveName &&
                entry.descriptiveName !== entry.productName && (
                  <Typography
                    variant="body2"
                    sx={{
                      color: 'text.secondary',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      display: '-webkit-box',
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: 'vertical',
                    }}
                  >
                    {entry.descriptiveName}
                  </Typography>
                )}
            </Box>

            {specs.length > 0 && (
              <Box
                sx={{
                  display: 'flex',
                  flexWrap: 'wrap',
                  gap: '12px 24px',
                  pt: 1.5,
                  borderTop: borders.divider,
                }}
              >
                {specs.map((spec) => (
                  <Box key={spec.label} sx={{ minWidth: 0 }}>
                    <Typography
                      variant="caption"
                      sx={{
                        color: 'text.secondary',
                        display: 'block',
                      }}
                    >
                      {spec.label}
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        fontWeight: 500,
                        color: 'text.primary',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        ...(spec.mono ? { fontFamily: fontFamilies.mono } : {}),
                      }}
                    >
                      {spec.value}
                    </Typography>
                  </Box>
                ))}
              </Box>
            )}
          </ContentSection>
        </CardWrapper>
      </Link>

      {/* Multi-photo Navigation Controls (sibling outside Link to avoid button-in-anchor nesting) */}
      {hasPhoto && photos.length > 1 && (
        <>
          <IconButton
            aria-label="Previous photo"
            onClick={handlePrevPhoto}
            size="small"
            sx={{
              position: 'absolute',
              left: 8,
              top: 92,
              transform: 'translateY(-50%)',
              zIndex: 3,
              width: 28,
              height: 28,
              p: 0,
              backgroundColor: fleetColors.surface,
              color: fleetColors.textPrimary,
              border: borders.default,
              boxShadow: elevations.card,
              '&:hover': {
                backgroundColor: fleetColors.surfaceTonal,
              },
            }}
          >
            <ChevronLeftIcon
              sx={{
                fontSize: iconSizes.inline,
                flexShrink: 0,
                display: 'block',
              }}
            />
          </IconButton>
          <IconButton
            aria-label="Next photo"
            onClick={handleNextPhoto}
            size="small"
            sx={{
              position: 'absolute',
              right: 8,
              top: 92,
              transform: 'translateY(-50%)',
              zIndex: 3,
              width: 28,
              height: 28,
              p: 0,
              backgroundColor: fleetColors.surface,
              color: fleetColors.textPrimary,
              border: borders.default,
              boxShadow: elevations.card,
              '&:hover': {
                backgroundColor: fleetColors.surfaceTonal,
              },
            }}
          >
            <ChevronRightIcon
              sx={{
                fontSize: iconSizes.inline,
                flexShrink: 0,
                display: 'block',
              }}
            />
          </IconButton>
        </>
      )}
    </Box>
  );
};

const LegacyCardWrapper = styled(Card)`
  display: flex;
  height: 100%;
  border: ${borders.default};
  border-radius: ${radii.md};
  background-color: ${fleetColors.surface};
  box-shadow: ${elevations.card};
  transition:
    transform 0.2s ease-in-out,
    border-color 0.2s ease-in-out;
  cursor: pointer;
  text-decoration: none;
  color: inherit;

  &:hover {
    transform: translateY(-3px);
    border-color: ${fleetColors.borderHover};
  }
`;

const LegacyImageSection = styled(Box)`
  width: 140px;
  min-width: 140px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(
    135deg,
    ${fleetColors.surfaceTonal} 0%,
    ${fleetColors.primaryContainer} 100%
  );
  border-right: ${borders.default};
  border-top-left-radius: ${radii.md};
  border-bottom-left-radius: ${radii.md};
`;

const getLegacyPlmStatusColor = (status?: string) => {
  switch (status?.toUpperCase()) {
    case PlmStatus.GA:
      return {
        bg: colors.green[50],
        text: colors.green[700],
        border: colors.green[200],
      };
    case PlmStatus.LA:
      return {
        bg: colors.orange[50],
        text: colors.orange[700],
        border: colors.orange[200],
      };
    case PlmStatus.NPI:
      return {
        bg: colors.blue[50],
        text: colors.blue[700],
        border: colors.blue[200],
      };
    default:
      return {
        bg: colors.grey[50],
        text: colors.grey[700],
        border: colors.grey[200],
      };
  }
};

const LegacyProductCatalogueCardView = ({
  entries,
  isLoading,
  pagination,
  onPaginationChange,
  totalCount,
}: ProductCatalogueCardViewProps) => {
  if (isLoading) {
    return (
      <Box sx={{ mt: 2 }}>
        <Grid container spacing={3}>
          {Array.from({ length: pagination.pageSize }).map((_, idx) => (
            <Grid item xs={12} md={6} key={idx}>
              <Card variant="surface" sx={{ display: 'flex', height: 160 }}>
                <Skeleton variant="rectangular" width={140} height="100%" />
                <Box
                  sx={{
                    flexGrow: 1,
                    p: 2,
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'space-between',
                  }}
                >
                  <Box>
                    <Skeleton variant="text" width="60%" height={28} />
                    <Skeleton variant="text" width="40%" height={20} />
                  </Box>
                  <Box sx={{ display: 'flex', gap: 2 }}>
                    <Skeleton variant="rectangular" width={80} height={24} />
                    <Skeleton variant="rectangular" width={80} height={24} />
                    <Skeleton variant="rectangular" width={80} height={24} />
                  </Box>
                </Box>
              </Card>
            </Grid>
          ))}
        </Grid>
      </Box>
    );
  }

  if (entries.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          py: 8,
          px: 2,
          border: `1px dashed ${colors.grey[300]}`,
          borderRadius: radii.md,
          backgroundColor: colors.grey[50],
        }}
      >
        <DevicesIcon sx={{ fontSize: 48, color: colors.grey[400], mb: 2 }} />
        <Typography
          variant="h6"
          sx={{ color: colors.grey[800], mb: 1, fontWeight: 600 }}
        >
          No catalog entries found
        </Typography>
        <Typography
          variant="body2"
          sx={{ color: colors.grey[600], textAlign: 'center' }}
        >
          Try adjusting your search or filters to find what you&apos;re looking
          for.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Grid container spacing={3}>
        {entries.map((entry) => (
          <Grid item xs={12} md={6} key={entry.productCatalogId}>
            <Link
              to={`/ui/fleet/catalog/${entry.productCatalogId}`}
              state={{
                navigatedFromLink: getPathnameWithParams(),
              }}
              style={{ textDecoration: 'none', color: 'inherit' }}
            >
              <LegacyCardWrapper
                data-testid={`legacy-catalog-card-${entry.productCatalogId}`}
              >
                <LegacyImageSection>
                  {getProductIcon(entry.productType)}
                </LegacyImageSection>
                <ContentSection>
                  <Box>
                    <Box
                      sx={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'flex-start',
                        mb: 0.5,
                      }}
                    >
                      <Typography
                        variant="h6"
                        sx={{
                          fontSize: fontSizes.title,
                          fontWeight: 600,
                          color: colors.grey[900],
                          lineHeight: 1.2,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                        }}
                      >
                        {entry.productName || entry.productCatalogId}
                      </Typography>
                      {entry.fleetPlmStatus && (
                        <Chip
                          label={entry.fleetPlmStatus}
                          size="small"
                          sx={{
                            ml: 1,
                            fontWeight: 600,
                            fontSize: fontSizes.caption,
                            height: 20,
                            backgroundColor: getLegacyPlmStatusColor(
                              entry.fleetPlmStatus,
                            ).bg,
                            color: getLegacyPlmStatusColor(entry.fleetPlmStatus)
                              .text,
                            border: `1px solid ${
                              getLegacyPlmStatusColor(entry.fleetPlmStatus)
                                .border
                            }`,
                          }}
                        />
                      )}
                    </Box>
                    <Typography
                      variant="caption"
                      sx={{
                        color: colors.grey[600],
                        display: 'block',
                        mb: 1.5,
                        fontFamily: fontFamilies.mono,
                      }}
                    >
                      ID: {entry.productCatalogId}
                    </Typography>
                    {entry.descriptiveName && (
                      <Typography
                        variant="body2"
                        sx={{
                          color: colors.grey[700],
                          mb: 2,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                          fontSize: fontSizes.label,
                        }}
                      >
                        {entry.descriptiveName}
                      </Typography>
                    )}
                  </Box>

                  <Box
                    sx={{
                      display: 'flex',
                      flexWrap: 'wrap',
                      gap: 3,
                      pt: 1.5,
                      borderTop: `1px solid ${colors.grey[100]}`,
                    }}
                  >
                    {entry.gpn && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          GPN
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: fontSizes.label,
                          }}
                        >
                          {entry.gpn}
                        </Typography>
                      </Box>
                    )}
                    {entry.resourceType && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          Resource Type
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: fontSizes.label,
                          }}
                        >
                          {entry.resourceType}
                        </Typography>
                      </Box>
                    )}
                    {entry.r11n && entry.r11n.length > 0 && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          R11N
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: fontSizes.label,
                          }}
                        >
                          {entry.r11n.join(', ')}
                        </Typography>
                      </Box>
                    )}
                    {entry.unitCost !== undefined &&
                      entry.unitCost !== '' &&
                      entry.unitCost !== '0' && (
                        <Box>
                          <Typography
                            variant="caption"
                            sx={{ color: colors.grey[500], display: 'block' }}
                          >
                            Unit Cost
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              color: colors.grey[800],
                              fontSize: fontSizes.label,
                            }}
                          >
                            ${entry.unitCost}
                          </Typography>
                        </Box>
                      )}
                    {entry.numberOfDevicesPerRack !== undefined &&
                      entry.numberOfDevicesPerRack !== 0 && (
                        <Box>
                          <Typography
                            variant="caption"
                            sx={{ color: colors.grey[500], display: 'block' }}
                          >
                            Per Rack
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              color: colors.grey[800],
                              fontSize: fontSizes.label,
                            }}
                          >
                            {entry.numberOfDevicesPerRack}
                          </Typography>
                        </Box>
                      )}
                    {entry.cpuType && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          CPU Type
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: fontSizes.label,
                          }}
                        >
                          {entry.cpuType}
                        </Typography>
                      </Box>
                    )}
                    {entry.cpuNumPerVm !== undefined &&
                      entry.cpuNumPerVm !== 0 && (
                        <Box>
                          <Typography
                            variant="caption"
                            sx={{ color: colors.grey[500], display: 'block' }}
                          >
                            CPU / VM
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              color: colors.grey[800],
                              fontSize: fontSizes.label,
                            }}
                          >
                            {entry.cpuNumPerVm}
                          </Typography>
                        </Box>
                      )}
                    {entry.memoryGbPerVm !== undefined &&
                      entry.memoryGbPerVm !== 0 && (
                        <Box>
                          <Typography
                            variant="caption"
                            sx={{ color: colors.grey[500], display: 'block' }}
                          >
                            Memory / VM
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              color: colors.grey[800],
                              fontSize: fontSizes.label,
                            }}
                          >
                            {entry.memoryGbPerVm} GB
                          </Typography>
                        </Box>
                      )}
                  </Box>
                </ContentSection>
              </LegacyCardWrapper>
            </Link>
          </Grid>
        ))}
      </Grid>

      <TablePagination
        component="div"
        count={totalCount}
        page={pagination.pageIndex}
        onPageChange={(_, newPage) => {
          onPaginationChange((prev) => ({ ...prev, pageIndex: newPage }));
        }}
        rowsPerPage={pagination.pageSize}
        onRowsPerPageChange={(e) => {
          const newPageSize = parseInt(e.target.value, 10);
          onPaginationChange((prev) => ({
            ...prev,
            pageIndex: 0,
            pageSize: newPageSize,
          }));
        }}
        rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
      />
    </Box>
  );
};

export const ProductCatalogueCardView = ({
  entries,
  isLoading,
  pagination,
  onPaginationChange,
  totalCount,
}: ProductCatalogueCardViewProps) => {
  const isRevampEnabled = useProductCatalogRevamp();
  if (!isRevampEnabled) {
    return (
      <LegacyProductCatalogueCardView
        entries={entries}
        isLoading={isLoading}
        pagination={pagination}
        onPaginationChange={onPaginationChange}
        totalCount={totalCount}
      />
    );
  }

  if (isLoading) {
    return (
      <Box sx={{ mt: 2 }}>
        <Grid container spacing={2.5}>
          {Array.from({ length: Math.min(pagination.pageSize, 12) }).map(
            (_, idx) => (
              <Grid item xs={12} sm={6} md={4} lg={3} key={idx}>
                <Card
                  variant="surface"
                  sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    height: 360,
                  }}
                >
                  <Skeleton variant="rectangular" width="100%" height={184} />
                  <Box
                    sx={{
                      flexGrow: 1,
                      p: 2,
                      display: 'flex',
                      flexDirection: 'column',
                      justifyContent: 'space-between',
                    }}
                  >
                    <Box>
                      <Skeleton variant="text" width="35%" height={16} />
                      <Skeleton variant="text" width="75%" height={26} />
                      <Skeleton variant="text" width="90%" height={18} />
                    </Box>
                    <Skeleton
                      variant="rectangular"
                      width="100%"
                      height={72}
                      sx={{ borderRadius: radii.sm }}
                    />
                  </Box>
                </Card>
              </Grid>
            ),
          )}
        </Grid>
      </Box>
    );
  }

  if (entries.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          py: 8,
          px: 2,
          border: `1px dashed ${colors.grey[300]}`,
          borderRadius: radii.md,
          backgroundColor: colors.grey[50],
        }}
      >
        <DevicesIcon sx={{ fontSize: 48, color: colors.grey[400], mb: 2 }} />
        <Typography
          variant="h6"
          sx={{ color: colors.grey[800], mb: 1, fontWeight: 600 }}
        >
          No catalog entries found
        </Typography>
        <Typography
          variant="body2"
          sx={{ color: colors.grey[600], textAlign: 'center' }}
        >
          Try adjusting your search or filters to find what you&apos;re looking
          for.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ mt: 1 }}>
      <Grid container spacing={2.5}>
        {entries.map((entry) => (
          <Grid item xs={12} sm={6} md={4} lg={3} key={entry.productCatalogId}>
            <CatalogEntryCard entry={entry} />
          </Grid>
        ))}
      </Grid>

      <TablePagination
        component="div"
        count={totalCount}
        page={pagination.pageIndex}
        onPageChange={(_, newPage) => {
          onPaginationChange((prev) => ({ ...prev, pageIndex: newPage }));
        }}
        rowsPerPage={pagination.pageSize}
        onRowsPerPageChange={(e) => {
          const newPageSize = parseInt(e.target.value, 10);
          onPaginationChange((prev) => ({
            ...prev,
            pageIndex: 0,
            pageSize: newPageSize,
          }));
        }}
        rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
      />
    </Box>
  );
};
