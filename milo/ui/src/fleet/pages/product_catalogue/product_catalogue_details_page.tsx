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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import {
  Alert,
  Box,
  IconButton,
  Link,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_Row,
} from 'material-react-table';
import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import {
  CATALOG_PATH,
  generateCatalogDetailsURL,
} from '@/fleet/constants/paths';
import { useProductCatalogRevamp } from '@/fleet/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { colors } from '@/fleet/theme/colors';
import {
  focusRingSx,
  iconButtonSx,
  surfaceCardClippedSx,
} from '@/fleet/theme/recipes';
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
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { CatalogPhotoEditDialog } from './catalog_edit_prompt_dialog';
import {
  arePhotoListsEqual,
  buildCatalogPhotoEditPrompt,
  MAX_PHOTOS_PER_ENTRY,
  validatePhotoDimensions,
  validatePhotoUrlInput,
} from './catalog_edit_validation';
import { OrderForm } from './order_form';
import { CategoryIllustration } from './product_catalogue_card_view';
import { COLUMNS } from './product_catalogue_columns';
import { UnifiedProductCatalogEntry } from './types';
import { UnsavedChangesToast } from './unsaved_changes_toast';
import { useProductCatalogDetailsData } from './use_product_catalog_details_data';

interface DetailRow {
  readonly key: string;
  readonly rawValue: unknown;
}

interface DetailValueCellProps {
  readonly row: MRT_Row<DetailRow>;
  readonly entry: UnifiedProductCatalogEntry | undefined;
  readonly isRevampEnabled?: boolean;
}

const GCE_ONLY_ACCESSORS = new Set<keyof UnifiedProductCatalogEntry>([
  'cpuType',
  'cpuNumPerVm',
  'memoryGbPerVm',
]);

const HARDWARE_ONLY_ACCESSORS = new Set<keyof UnifiedProductCatalogEntry>([
  'gpn',
  'resourceType',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
]);

const getPlmStatusTone = (status?: string): StatusTone => {
  switch (status?.toUpperCase()) {
    case 'GA':
    case 'ACTIVE':
      return 'success';
    case 'LA':
      return 'warning';
    case 'NPI':
      return 'info';
    default:
      return 'neutral';
  }
};

const resolveProductType = (
  productType?: string,
  productCatalogId?: string,
): string => {
  const explicit = (productType || '').trim().toLowerCase();
  if (explicit) return explicit;
  const upperId = (productCatalogId || '').trim().toUpperCase();
  if (upperId.startsWith('ATB-')) return 'android-testbed';
  if (upperId.startsWith('TB-')) return 'os-testbed';
  if (upperId.startsWith('PH-')) return 'hardware';
  if (upperId.startsWith('PP-')) return 'peripherals';
  if (upperId.startsWith('GCE-')) return 'gce';
  return '';
};

const formatProductCategory = (
  productType?: string,
  productCatalogId?: string,
) => {
  const prodType = resolveProductType(productType, productCatalogId);
  switch (prodType) {
    case 'android-testbed':
      return 'Android Testbed';
    case 'os-testbed':
      return 'OS Testbed';
    case 'hardware':
      return 'DUT & Host Hardware';
    case 'peripherals':
      return 'Lab Peripheral & Fixture';
    case 'gce':
      return 'GCE Virtual Machine';
    default:
      return productType || 'Catalog Item';
  }
};

const formatUnitCost = (unitCost?: string) => {
  if (!unitCost || unitCost === '0') return '—';
  const trimmed = unitCost.trim();
  if (!trimmed) return '—';
  if (trimmed.startsWith('$')) return trimmed;
  const numeric = Number(trimmed.replace(/,/g, ''));
  if (Number.isNaN(numeric)) {
    return trimmed;
  }
  if (numeric > 0) {
    return `$${numeric.toLocaleString('en-US')}`;
  }
  return '—';
};

const DetailValueCell = ({
  row,
  entry,
  isRevampEnabled = false,
}: DetailValueCellProps) => {
  const fieldKey = row.original.key;
  const rawValue = row.original.rawValue;

  const col = COLUMNS.find((c) => c.header === fieldKey);
  if (!col) return <>{String(rawValue ?? '')}</>;

  const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;

  if (accessor === 'productCatalogId') {
    if (!isRevampEnabled) {
      return <>{String(rawValue ?? '')}</>;
    }
    return (
      <Typography
        variant="body2"
        sx={{
          fontFamily: fontFamilies.mono,
          fontWeight: 500,
        }}
      >
        {String(rawValue ?? '')}
      </Typography>
    );
  }

  if (isRevampEnabled && accessor === 'fleetPlmStatus') {
    const statusStr = String(rawValue ?? '').trim();
    if (!statusStr) return <>—</>;
    return (
      <StatusPill tone={getPlmStatusTone(statusStr)} size="compact" dot={false}>
        {statusStr}
      </StatusPill>
    );
  }

  if (isRevampEnabled && accessor === 'unitCost') {
    return <>{formatUnitCost(String(rawValue ?? ''))}</>;
  }

  if (col.Cell && entry) {
    if (
      isRevampEnabled &&
      (!rawValue ||
        (Array.isArray(rawValue) &&
          rawValue.filter((v) => String(v ?? '').trim()).length === 0))
    ) {
      return <>—</>;
    }
    // SAFETY: We cast the column's Cell renderer to a component accepting our mocked
    // props. This is safe because our cell renderers in COLUMNS only rely on
    // cell.getValue() and row.original, which we mock here with compatible shapes.
    const CellRenderer = col.Cell as unknown as React.ComponentType<{
      cell: { getValue: () => unknown };
      row: { original: UnifiedProductCatalogEntry };
      column: typeof col;
      table: Record<string, never>;
    }>;

    return (
      <CellRenderer
        cell={{
          getValue: () => rawValue,
        }}
        row={{
          original: entry,
        }}
        column={col}
        table={{}}
      />
    );
  }

  if (Array.isArray(rawValue)) {
    const joined = rawValue.join(', ').trim();
    return <>{joined || (isRevampEnabled ? '—' : '')}</>;
  }

  const strValue = String(rawValue ?? '').trim();
  if (!strValue && isRevampEnabled) {
    return <>—</>;
  }

  return <>{String(rawValue ?? '')}</>;
};

const useNavigatedFromLink = () => {
  const { state } = useLocation();

  const [navigatedFromLink, setNavigatedFromLink] = useState<
    string | undefined
  >(undefined);

  // state from useLocation is lost on rerenders so we need to save it in a react state
  useEffect(() => {
    if (state) {
      setNavigatedFromLink(state.navigatedFromLink);
    }
  }, [state]);

  return navigatedFromLink;
};

interface CatalogDetailsHeroProps {
  readonly entry: UnifiedProductCatalogEntry;
  readonly stagedPhotos: readonly string[];
  readonly onOpenPhotoEditor: () => void;
  readonly onPhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

const CatalogDetailsHero = ({
  entry,
  stagedPhotos,
  onOpenPhotoEditor,
  onPhotoDimensionsEvaluated,
}: CatalogDetailsHeroProps) => {
  const [activePhotoIdx, setActivePhotoIdx] = useState(0);
  const [failedUris, setFailedUris] = useState<Record<string, boolean>>({});

  const photos = stagedPhotos.filter((uri) => Boolean(uri) && !failedUris[uri]);
  const safePhotoIdx = photos.length > 0 ? activePhotoIdx % photos.length : 0;
  const currentPhotoUri = photos[safePhotoIdx];
  const hasPhoto = photos.length > 0;
  const resolvedProductType = resolveProductType(
    entry.productType,
    entry.productCatalogId,
  );
  const isGce = resolvedProductType === 'gce';
  const formattedCost = formatUnitCost(entry.unitCost);

  const summaryMetrics = isGce
    ? [
        {
          label: 'CPU Architecture',
          value: entry.cpuType || '—',
          mono: false,
        },
        {
          label: 'VM Compute Profile',
          value: entry.cpuNumPerVm
            ? `${entry.cpuNumPerVm} vCPU • ${entry.memoryGbPerVm ?? 0} GB`
            : '—',
          mono: false,
        },
        ...(formattedCost !== '—'
          ? [
              {
                label: 'Estimated Unit Cost',
                value: formattedCost,
                mono: false,
              },
            ]
          : []),
        {
          label: 'Fulfillment Model',
          value: 'Cloud Virtual Instance',
          mono: false,
        },
      ]
    : [
        {
          label: 'Google Part Number (GPN)',
          value: entry.gpn || '—',
          mono: Boolean(entry.gpn),
        },
        {
          label: 'Resource Type',
          value: entry.resourceType || '—',
          mono: false,
        },
        ...(formattedCost !== '—'
          ? [
              {
                label: 'Estimated Unit Cost',
                value: formattedCost,
                mono: false,
              },
            ]
          : []),
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
  const r11nItems = (entry.r11n ?? []).map((val) => val.trim()).filter(Boolean);
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
      {/* Left Hardware Showcase Stage */}
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          borderRight: { xs: 'none', md: borders.divider },
          borderBottom: { xs: borders.divider, md: 'none' },
          backgroundColor: fleetColors.surface,
        }}
      >
        <Box
          data-testid="details-photo-stage"
          sx={{
            position: 'relative',
            flex: 1,
            minHeight: 210,
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            p: 1.5,
            overflow: 'hidden',
            background: `linear-gradient(135deg, ${fleetColors.surfaceTonal} 0%, ${fleetColors.primaryContainer} 100%)`,
          }}
        >
          {/* Edit Photo Trigger Overlaid on the Photo Stage (opens spacious modal) */}
          {!isGce && (
            <Tooltip title="Edit or upload reference photos">
              <IconButton
                size="small"
                aria-label="Edit photos"
                onClick={onOpenPhotoEditor}
                data-testid="toggle-photo-editor-btn"
                sx={{
                  position: 'absolute',
                  top: 8,
                  right: 8,
                  zIndex: 5,
                  width: 32,
                  height: 32,
                  backgroundColor: fleetColors.surface,
                  color: fleetColors.textPrimary,
                  border: borders.default,
                  boxShadow: elevations.card,
                  '&:hover': {
                    backgroundColor: fleetColors.surfaceTonal,
                    color: fleetColors.primary,
                  },
                }}
              >
                <EditOutlinedIcon sx={{ fontSize: iconSizes.control }} />
              </IconButton>
            </Tooltip>
          )}

          {hasPhoto ? (
            <>
              <Box
                component="img"
                src={currentPhotoUri}
                alt={`${entry.productName || entry.productCatalogId} hardware reference`}
                data-testid="details-photo-img"
                onLoad={(e) => {
                  const img = e.currentTarget as HTMLImageElement;
                  if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                    onPhotoDimensionsEvaluated(
                      currentPhotoUri,
                      img.naturalWidth,
                      img.naturalHeight,
                    );
                  }
                }}
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
                <>
                  <IconButton
                    aria-label="Previous photo"
                    onClick={() =>
                      setActivePhotoIdx((prev) => {
                        const idx = prev % photos.length;
                        return idx > 0 ? idx - 1 : photos.length - 1;
                      })
                    }
                    size="small"
                    sx={{
                      ...focusRingSx,
                      position: 'absolute',
                      left: 8,
                      top: '50%',
                      transform: 'translateY(-50%)',
                      zIndex: 3,
                      width: controlHeights.button,
                      height: controlHeights.button,
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
                    onClick={() =>
                      setActivePhotoIdx((prev) => {
                        const idx = prev % photos.length;
                        return idx < photos.length - 1 ? idx + 1 : 0;
                      })
                    }
                    size="small"
                    sx={{
                      ...focusRingSx,
                      position: 'absolute',
                      right: 8,
                      top: '50%',
                      transform: 'translateY(-50%)',
                      zIndex: 3,
                      width: controlHeights.button,
                      height: controlHeights.button,
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
            </>
          ) : (
            <Box
              data-testid="details-photo-fallback"
              sx={{
                display: 'flex',
                flex: 1,
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <CategoryIllustration
                productType={resolvedProductType}
                productCatalogId={entry.productCatalogId}
              />
            </Box>
          )}

          {/* Read-mode thumbnail strip overlaid at bottom of photo stage */}
          {photos.length > 1 && (
            <Box
              data-testid="details-photo-thumbnails"
              sx={{
                position: 'absolute',
                left: 8,
                right: 8,
                bottom: 8,
                zIndex: 3,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 0.75,
                pointerEvents: 'none',
              }}
            >
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.5,
                  pointerEvents: 'auto',
                  overflowX: 'auto',
                  maxWidth: 'calc(100% - 64px)',
                }}
              >
                {photos.map((uri, idx) => (
                  <Box
                    key={`thumb-${idx}`}
                    component="button"
                    type="button"
                    aria-label={`View photo ${idx + 1} of ${photos.length}`}
                    onClick={() => setActivePhotoIdx(idx)}
                    sx={{
                      ...focusRingSx,
                      p: 0,
                      width: 40,
                      height: 28,
                      flexShrink: 0,
                      borderRadius: radii.sm,
                      overflow: 'hidden',
                      cursor: 'pointer',
                      border:
                        idx === safePhotoIdx
                          ? `2px solid ${fleetColors.primary}`
                          : borders.default,
                      boxShadow: elevations.card,
                      backgroundColor: fleetColors.surface,
                    }}
                  >
                    <Box
                      component="img"
                      src={uri}
                      alt={`Thumbnail ${idx + 1}`}
                      sx={{
                        width: '100%',
                        height: '100%',
                        objectFit: 'cover',
                        display: 'block',
                      }}
                    />
                  </Box>
                ))}
              </Box>
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
                    fontWeight: 600,
                    lineHeight: 1,
                  }}
                >
                  {`${safePhotoIdx + 1} / ${photos.length}`}
                </Typography>
              </Box>
            </Box>
          )}
        </Box>
      </Box>

      {/* Right Hardware Identity & Key Summary Strip */}
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

        {/* Key Procurement Summary Strip */}
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
                sx={{
                  display: 'block',
                  color: 'text.secondary',
                  mb: 0.25,
                }}
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
          {!isGce && (
            <Box sx={{ minWidth: 0 }}>
              <Typography
                variant="caption"
                sx={{
                  display: 'block',
                  color: 'text.secondary',
                  mb: 0.25,
                }}
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
          )}
        </Box>
      </Box>
    </Box>
  );
};

export const ProductCatalogDetailsPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const navigatedFromLink = useNavigatedFromLink();
  const isRevampEnabled = useProductCatalogRevamp();

  const navigateToCatalogIfChanged = (newId: string) => {
    if (id !== newId) {
      navigate(generateCatalogDetailsURL(newId), {
        state: { navigatedFromLink },
      });
    }
  };

  const { error, isError, isLoading, entry } = useProductCatalogDetailsData(id);

  const originalPhotos = useMemo(
    () => entry?.photoDataUris ?? [],
    [entry?.photoDataUris],
  );

  const [isPhotoEditorOpen, setIsPhotoEditorOpen] = useState(false);
  const [stagedPhotosOverride, setStagedPhotosOverride] = useState<
    readonly string[] | undefined
  >(undefined);
  const [photoUrlDraft, setPhotoUrlDraft] = useState('');
  const [photoInputError, setPhotoInputError] = useState<string | undefined>(
    undefined,
  );
  const [photoDimensionErrors, setPhotoDimensionErrors] = useState<
    Record<string, string>
  >({});
  const [copiedPrompt, setCopiedPrompt] = useState(false);

  // Reset staged edits when switching catalog entries.
  useEffect(() => {
    setStagedPhotosOverride(undefined);
    setIsPhotoEditorOpen(false);
    setPhotoUrlDraft('');
    setPhotoInputError(undefined);
    setPhotoDimensionErrors({});
    setCopiedPrompt(false);
  }, [id]);

  const stagedPhotos = stagedPhotosOverride ?? originalPhotos;
  const hasPhotoChanges =
    entry !== undefined && !arePhotoListsEqual(originalPhotos, stagedPhotos);
  const dirtyCount = hasPhotoChanges ? 1 : 0;

  const activeDimensionErrorMessages = stagedPhotos
    .filter(
      (uri) =>
        Boolean(photoDimensionErrors[uri]) && !originalPhotos.includes(uri),
    )
    .map((uri) => photoDimensionErrors[uri]);
  const validationErrorCount = activeDimensionErrorMessages.length;
  const firstValidationErrorMessage = activeDimensionErrorMessages[0];

  const handleStagePhotoUrl = () => {
    const err = validatePhotoUrlInput(photoUrlDraft, stagedPhotos);
    if (err) {
      setPhotoInputError(err);
      return;
    }
    const trimmed = photoUrlDraft.trim();
    setStagedPhotosOverride([...stagedPhotos, trimmed]);
    setPhotoUrlDraft('');
    setPhotoInputError(undefined);
  };

  const handleRemoveStagedPhoto = (idx: number) => {
    const removedUri = stagedPhotos[idx];
    const next = stagedPhotos.filter((_, i) => i !== idx);
    setStagedPhotosOverride(next);
    setPhotoInputError(undefined);
    if (removedUri) {
      setPhotoDimensionErrors((prev) => {
        if (!prev[removedUri]) return prev;
        const copy = { ...prev };
        delete copy[removedUri];
        return copy;
      });
    }
  };

  const handleSetPrimaryPhoto = (idx: number) => {
    if (idx <= 0 || idx >= stagedPhotos.length) return;
    const target = stagedPhotos[idx];
    const rest = stagedPhotos.filter((_, i) => i !== idx);
    setStagedPhotosOverride([target, ...rest]);
  };

  const handleDiscardEdits = () => {
    setStagedPhotosOverride(undefined);
    setPhotoUrlDraft('');
    setPhotoInputError(undefined);
    setPhotoDimensionErrors({});
    setCopiedPrompt(false);
  };

  const handlePhotoDimensionsEvaluated = (
    uri: string,
    width: number,
    height: number,
  ) => {
    const dimErr = validatePhotoDimensions(width, height);
    setPhotoDimensionErrors((prev) => {
      if (dimErr ? prev[uri] === dimErr : !prev[uri]) return prev;
      const next = { ...prev };
      if (dimErr) {
        next[uri] = dimErr;
      } else {
        delete next[uri];
      }
      return next;
    });
  };

  const generatedPrompt = useMemo(() => {
    if (!entry) return '';
    return buildCatalogPhotoEditPrompt(entry, stagedPhotos);
  }, [entry, stagedPhotos]);

  useEffect(() => {
    setCopiedPrompt(false);
  }, [generatedPrompt]);

  const handleCopyPrompt = async () => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(generatedPrompt);
        setCopiedPrompt(true);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = generatedPrompt;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        const copied = document.execCommand?.('copy') ?? true;
        document.body.removeChild(textarea);
        setCopiedPrompt(copied);
      }
    } catch {
      setCopiedPrompt(false);
    }
  };

  const columns = useMemo<MRT_ColumnDef<DetailRow>[]>(
    () => [
      {
        accessorKey: 'key',
        header: 'Field',
        size: 210,
        minSize: 210,
        maxSize: 210,
        grow: 0,
        muiTableBodyCellProps: {
          sx: {
            fontWeight: 500,
            color: 'text.secondary',
            ...(isRevampEnabled
              ? {
                  py: 0,
                  height: 44,
                  minHeight: 44,
                  maxHeight: 44,
                  boxSizing: 'border-box',
                  verticalAlign: 'middle',
                }
              : {}),
          },
        },
      },
      {
        accessorKey: 'rawValue',
        header: 'Value',
        size: 240,
        grow: 1,
        muiTableBodyCellProps: {
          sx: {
            ...(isRevampEnabled
              ? {
                  minWidth: 0,
                  py: 0,
                  height: 44,
                  minHeight: 44,
                  maxHeight: 44,
                  boxSizing: 'border-box',
                  verticalAlign: 'middle',
                }
              : {}),
          },
        },
        Cell: ({ row }: { readonly row: MRT_Row<DetailRow> }) => (
          <DetailValueCell
            row={row}
            entry={entry}
            isRevampEnabled={isRevampEnabled}
          />
        ),
      },
    ],
    [entry, isRevampEnabled],
  );

  const data = useMemo<DetailRow[]>(() => {
    if (!entry) return [];
    const resolvedType = resolveProductType(
      entry.productType,
      entry.productCatalogId,
    );
    const isGce = resolvedType === 'gce';
    const filteredCols = isRevampEnabled
      ? COLUMNS.filter((col) => {
          const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;
          return isGce
            ? !HARDWARE_ONLY_ACCESSORS.has(accessor)
            : !GCE_ONLY_ACCESSORS.has(accessor);
        })
      : COLUMNS;

    return filteredCols.map((col) => {
      const accessor = col.accessorKey as keyof UnifiedProductCatalogEntry;
      const val =
        isRevampEnabled && accessor === 'productType'
          ? entry.productType || resolvedType
          : entry[accessor];
      return {
        key: col.header as string,
        rawValue: val,
      };
    });
  }, [entry, isRevampEnabled]);

  const table = useFCDataTable({
    columns,
    data,
    layoutMode: isRevampEnabled ? 'semantic' : 'grid',
    enableRowVirtualization: false,
    enablePagination: false,
    enableColumnActions: false,
    enableColumnResizing: !isRevampEnabled,
    enableSorting: false,
    enableTopToolbar: false,
    enableBottomToolbar: false,
    enableTableHead: true,
    enableStickyHeader: true,
    muiTableProps: isRevampEnabled
      ? {
          sx: {
            tableLayout: 'fixed',
            width: '100%',
            '&& .MuiTableCell-root:first-of-type': {
              width: 210,
              minWidth: 210,
              maxWidth: 210,
              paddingLeft: '16px',
              paddingRight: '12px',
              boxSizing: 'border-box',
            },
            '&& .MuiTableCell-root:last-of-type': {
              width: 'auto',
              minWidth: 0,
              paddingLeft: '16px',
              paddingRight: '16px',
              boxSizing: 'border-box',
            },
          },
        }
      : undefined,
    muiTablePaperProps: isRevampEnabled
      ? {
          elevation: 0,
          sx: {
            border: 'none',
            borderRadius: 0,
            boxShadow: 'none',
          },
        }
      : undefined,
    muiTableContainerProps: {
      sx: {
        maxWidth: '100%',
        overflowX: 'hidden',
        margin: isRevampEnabled ? 0 : '24px 0',
        ...(isRevampEnabled ? { maxHeight: 'none', overflowY: 'visible' } : {}),
      },
    },
  });

  if (isError) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">
          Something went wrong:{' '}
          {getErrorMessage(error, 'fetch product catalog entry')}
        </Alert>
      </Box>
    );
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', my: 4 }}>
        <CentralizedProgress data-testid="loading-spinner" />
      </Box>
    );
  }

  if (!isRevampEnabled) {
    return (
      <Box sx={{ p: 3 }} data-testid="legacy-catalog-details-view">
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            gap: 1,
            mb: 3,
          }}
        >
          <IconButton
            aria-label="go back"
            onClick={() => {
              if (navigatedFromLink) {
                navigate(navigatedFromLink);
              } else {
                navigate(CATALOG_PATH);
              }
            }}
            sx={{ mr: 2 }}
          >
            <ArrowBackIcon />
          </IconButton>
          <Typography variant="h4" component="h1" sx={{ whiteSpace: 'nowrap' }}>
            Product Catalog Entry:
          </Typography>
          <TextField
            key={id}
            variant="standard"
            defaultValue={id}
            slotProps={{ htmlInput: { sx: { fontSize: 24 } } }}
            fullWidth
            onBlur={(e) => {
              navigateToCatalogIfChanged(e.target.value);
            }}
            onKeyDown={(e) => {
              const target = e.target as HTMLInputElement;
              if (e.key === 'Enter') {
                navigateToCatalogIfChanged(target.value);
              }
            }}
          />
        </Box>

        {!entry ? (
          <AlertWithFeedback
            title="Entry not found!"
            bugErrorMessage={`Product Catalog Entry not found: ${id}`}
          >
            <p>
              The product catalog entry <code>{id}</code> you are looking for
              was not found.
            </p>
          </AlertWithFeedback>
        ) : (
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' },
              gap: 3,
              alignItems: 'start',
            }}
          >
            <Box sx={{ overflowX: 'auto', order: { xs: 2, lg: 1 } }}>
              <MaterialReactTable table={table} />
            </Box>
            <Box sx={{ order: { xs: 1, lg: 2 } }}>
              <OrderForm entry={entry} />
            </Box>
          </Box>
        )}
      </Box>
    );
  }

  const resolvedProductType = entry
    ? resolveProductType(entry.productType, entry.productCatalogId)
    : '';

  return (
    <Box sx={{ p: 3, pb: dirtyCount > 0 || validationErrorCount > 0 ? 12 : 3 }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          gap: 1,
          mb: 2.5,
        }}
      >
        <IconButton
          aria-label="go back"
          onClick={() => {
            if (navigatedFromLink) {
              navigate(navigatedFromLink);
            } else {
              navigate(CATALOG_PATH);
            }
          }}
          sx={{ ...iconButtonSx, mr: 1 }}
        >
          <ArrowBackIcon
            sx={{ fontSize: iconSizes.inline, display: 'block' }}
          />
        </IconButton>
        <Typography
          variant="h5"
          component="h1"
          sx={{
            whiteSpace: 'nowrap',
            fontWeight: 700,
            color: fleetColors.textPrimary,
          }}
        >
          Product Catalog Entry:
        </Typography>
        <TextField
          key={id}
          variant="standard"
          defaultValue={id}
          slotProps={{
            htmlInput: {
              sx: {
                fontSize: fontSizes.title,
                fontWeight: 600,
                fontFamily: fontFamilies.mono,
              },
            },
          }}
          sx={{ maxWidth: 280 }}
          onBlur={(e) => {
            navigateToCatalogIfChanged(e.target.value);
          }}
          onKeyDown={(e) => {
            const target = e.target as HTMLInputElement;
            if (e.key === 'Enter') {
              navigateToCatalogIfChanged(target.value);
            }
          }}
        />
      </Box>

      {!entry ? (
        <AlertWithFeedback
          title="Entry not found!"
          bugErrorMessage={`Product Catalog Entry not found: ${id}`}
        >
          <p>
            The product catalog entry <code>{id}</code> you are looking for was
            not found.
          </p>
        </AlertWithFeedback>
      ) : (
        <>
          <CatalogDetailsHero
            key={entry.productCatalogId}
            entry={entry}
            stagedPhotos={stagedPhotos}
            onOpenPhotoEditor={() => setIsPhotoEditorOpen(true)}
            onPhotoDimensionsEvaluated={handlePhotoDimensionsEvaluated}
          />
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', lg: '7fr 5fr' },
              gap: 3,
              alignItems: 'start',
            }}
          >
            {/* Left Column: Specifications Table (with clean Field | Value header) */}
            <Box
              data-testid="catalog-specs-card"
              sx={{
                ...surfaceCardClippedSx,
                order: { xs: 2, lg: 1 },
              }}
            >
              <Box sx={{ overflowX: 'auto' }}>
                <MaterialReactTable table={table} />
              </Box>
            </Box>

            <Box sx={{ order: { xs: 1, lg: 2 } }}>
              <OrderForm entry={entry} />
            </Box>
          </Box>

          <CatalogPhotoEditDialog
            open={isPhotoEditorOpen}
            onClose={() => setIsPhotoEditorOpen(false)}
            onCopyPrompt={handleCopyPrompt}
            copiedPrompt={copiedPrompt}
            prompt={generatedPrompt}
            productCatalogId={entry.productCatalogId}
            productName={entry.productName || entry.descriptiveName}
            resolvedProductType={resolvedProductType}
            originalPhotos={originalPhotos}
            stagedPhotos={stagedPhotos}
            photoUrlDraft={photoUrlDraft}
            photoInputError={photoInputError}
            photoDimensionErrors={photoDimensionErrors}
            onPhotoUrlDraftChange={(val) => {
              setPhotoUrlDraft(val);
              if (photoInputError) setPhotoInputError(undefined);
            }}
            onStagePhotoUrl={handleStagePhotoUrl}
            onStageUploadedDataUrl={(dataUrl) => {
              if (stagedPhotos.length >= MAX_PHOTOS_PER_ENTRY) return;
              setStagedPhotosOverride([...stagedPhotos, dataUrl]);
              setPhotoInputError(undefined);
            }}
            onRemoveStagedPhoto={handleRemoveStagedPhoto}
            onSetPrimaryPhoto={handleSetPrimaryPhoto}
            onResetPhotos={handleDiscardEdits}
            onPhotoDimensionsEvaluated={handlePhotoDimensionsEvaluated}
          />

          {!isPhotoEditorOpen && (
            <UnsavedChangesToast
              dirtyCount={dirtyCount}
              validationErrorCount={validationErrorCount}
              errorSummaryText={firstValidationErrorMessage}
              productCatalogId={entry.productCatalogId}
              prompt={generatedPrompt}
              copiedPrompt={copiedPrompt}
              onDiscard={handleDiscardEdits}
              onCopyPrompt={handleCopyPrompt}
            />
          )}
        </>
      )}
    </Box>
  );
};

export function Component() {
  const { id = '' } = useParams();
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-product-catalog-details">
      <FleetHelmet
        pageTitle={`${id ? `${id} | ` : ''}Product Catalog Details`}
      />
      <RecoverableErrorBoundary key="fleet-product-catalog-details-page">
        <LoggedInBoundary>
          <ProductCatalogDetailsPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
