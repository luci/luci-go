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
import { Alert, Box, IconButton, TextField, Typography } from '@mui/material';
import { useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import {
  CATALOG_PATH,
  generateCatalogDetailsURL,
} from '@/fleet/constants/paths';
import { useProductCatalogRevamp } from '@/fleet/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { iconButtonSx, surfaceCardClippedSx } from '@/fleet/theme/recipes';
import {
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
} from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { resolveProductType } from './catalog_details_helpers';
import { CatalogDetailsHero } from './catalog_details_hero';
import { CatalogPhotoEditDialog } from './catalog_edit_prompt_dialog';
import { CatalogSpecsTable } from './catalog_specs_table';
import { OrderForm } from './order_form';
import { UnsavedChangesToast } from './unsaved_changes_toast';
import { useCatalogEditState } from './use_catalog_edit_state';
import { useProductCatalogDetailsData } from './use_product_catalog_details_data';

interface CatalogLocationState {
  readonly navigatedFromLink?: string;
}

interface CatalogEntryHeaderProps {
  readonly id: string;
  readonly isRevampEnabled: boolean;
  readonly onBack: () => void;
  readonly onIdCommit: (newId: string) => void;
}

const CatalogEntryHeader = ({
  id,
  isRevampEnabled,
  onBack,
  onIdCommit,
}: CatalogEntryHeaderProps) => (
  <Box
    sx={{
      display: 'flex',
      flexDirection: 'row',
      alignItems: 'center',
      gap: 1,
      mb: isRevampEnabled ? 2.5 : 3,
    }}
  >
    <IconButton
      aria-label="go back"
      onClick={onBack}
      sx={isRevampEnabled ? { ...iconButtonSx, mr: 1 } : { mr: 2 }}
    >
      <ArrowBackIcon
        sx={
          isRevampEnabled
            ? { fontSize: iconSizes.inline, display: 'block' }
            : undefined
        }
      />
    </IconButton>
    <Typography
      variant={isRevampEnabled ? 'h5' : 'h4'}
      component="h1"
      sx={{
        whiteSpace: 'nowrap',
        ...(isRevampEnabled
          ? { fontWeight: 700, color: fleetColors.textPrimary }
          : {}),
      }}
    >
      Product Catalog Entry:
    </Typography>
    <TextField
      key={id}
      variant="standard"
      defaultValue={id}
      fullWidth={!isRevampEnabled}
      slotProps={{
        htmlInput: {
          sx: isRevampEnabled
            ? {
                fontSize: fontSizes.title,
                fontWeight: 600,
                fontFamily: fontFamilies.mono,
              }
            : { fontSize: fontSizes.headline },
        },
      }}
      sx={isRevampEnabled ? { maxWidth: 280 } : undefined}
      onBlur={(e) => onIdCommit(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          onIdCommit((e.target as HTMLInputElement).value);
        }
      }}
    />
  </Box>
);

export const ProductCatalogDetailsPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const [navigatedFromLink] = useState<string | undefined>(
    () =>
      (location.state as CatalogLocationState | null | undefined)
        ?.navigatedFromLink,
  );
  const isRevampEnabled = useProductCatalogRevamp();

  const navigateToCatalogIfChanged = (newId: string) => {
    if (id !== newId) {
      navigate(generateCatalogDetailsURL(newId), {
        state: { navigatedFromLink },
      });
    }
  };

  const handleBack = () => {
    navigate(navigatedFromLink || CATALOG_PATH);
  };

  const { error, isError, isLoading, entry } = useProductCatalogDetailsData(id);
  const editState = useCatalogEditState(id, entry);

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

  const activeHeroEntry = isRevampEnabled
    ? (editState.displayEntry ?? entry)
    : entry;

  return (
    <Box
      data-testid={!isRevampEnabled ? 'legacy-catalog-details-view' : undefined}
      sx={{
        p: 3,
        pb:
          isRevampEnabled &&
          (editState.dirtyCount > 0 || editState.validationErrorCount > 0)
            ? 12
            : 3,
      }}
    >
      <CatalogEntryHeader
        id={id}
        isRevampEnabled={isRevampEnabled}
        onBack={handleBack}
        onIdCommit={navigateToCatalogIfChanged}
      />

      {!activeHeroEntry ? (
        <AlertWithFeedback
          title="Entry not found!"
          bugErrorMessage={`Product Catalog Entry not found: ${id}`}
        >
          <p>
            The product catalog entry <code>{id}</code> you are looking for was
            not found.
          </p>
        </AlertWithFeedback>
      ) : !isRevampEnabled ? (
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' },
            gap: 3,
            alignItems: 'start',
          }}
        >
          <Box sx={{ overflowX: 'auto', order: { xs: 2, lg: 1 } }}>
            <CatalogSpecsTable
              entry={entry}
              isRevampEnabled={false}
              isEditingSpecs={false}
              onToggleEditingSpecs={editState.handleToggleEditingSpecs}
              onStartEditingSpecs={editState.handleStartEditingSpecs}
              activeSpecDraft={editState.activeSpecDraft}
              baselineSpecDraft={editState.baselineSpecDraft}
              specErrors={editState.specErrors}
              onSpecFieldChange={editState.handleSpecFieldChange}
            />
          </Box>
          <Box sx={{ order: { xs: 1, lg: 2 } }}>
            <OrderForm entry={activeHeroEntry} />
          </Box>
        </Box>
      ) : (
        <>
          <CatalogDetailsHero
            key={activeHeroEntry.productCatalogId}
            entry={activeHeroEntry}
            stagedPhotos={editState.stagedPhotos}
            onOpenPhotoEditor={editState.handleOpenPhotoEditor}
            onPhotoDimensionsEvaluated={
              editState.handlePhotoDimensionsEvaluated
            }
          />
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', lg: '7fr 5fr' },
              gap: 3,
              alignItems: 'start',
            }}
          >
            <Box
              data-testid="catalog-specs-card"
              sx={{ ...surfaceCardClippedSx, order: { xs: 2, lg: 1 } }}
            >
              <Box sx={{ overflowX: 'auto' }}>
                <CatalogSpecsTable
                  entry={entry}
                  displayEntry={editState.displayEntry}
                  isRevampEnabled={isRevampEnabled}
                  isEditingSpecs={editState.isEditingSpecs}
                  onToggleEditingSpecs={editState.handleToggleEditingSpecs}
                  onStartEditingSpecs={editState.handleStartEditingSpecs}
                  onStopEditingSpecs={editState.handleStopEditingSpecs}
                  activeSpecDraft={editState.activeSpecDraft}
                  baselineSpecDraft={editState.baselineSpecDraft}
                  specErrors={editState.specErrors}
                  onSpecFieldChange={editState.handleSpecFieldChange}
                />
              </Box>
            </Box>
            <Box sx={{ order: { xs: 1, lg: 2 } }}>
              <OrderForm entry={activeHeroEntry} />
            </Box>
          </Box>

          <CatalogPhotoEditDialog
            open={editState.isPhotoEditorOpen}
            onClose={editState.handleClosePhotoEditor}
            onCopyPrompt={editState.handleCopyPrompt}
            copiedPrompt={editState.copiedPrompt}
            prompt={editState.generatedPrompt}
            productCatalogId={activeHeroEntry.productCatalogId}
            productName={
              activeHeroEntry.productName || activeHeroEntry.descriptiveName
            }
            resolvedProductType={resolveProductType(
              activeHeroEntry.productType,
              activeHeroEntry.productCatalogId,
            )}
            originalPhotos={editState.originalPhotos}
            stagedPhotos={editState.stagedPhotos}
            photoUrlDraft={editState.photoUrlDraft}
            photoInputError={editState.photoInputError}
            photoDimensionErrors={editState.photoDimensionErrors}
            onPhotoUrlDraftChange={editState.handlePhotoUrlDraftChange}
            onStagePhotoUrl={editState.handleStagePhotoUrl}
            onRemoveStagedPhoto={editState.handleRemoveStagedPhoto}
            onSetPrimaryPhoto={editState.handleSetPrimaryPhoto}
            onResetPhotos={editState.handleResetPhotos}
            onPhotoDimensionsEvaluated={
              editState.handlePhotoDimensionsEvaluated
            }
          />

          {!editState.isPhotoEditorOpen && (
            <UnsavedChangesToast
              dirtyCount={editState.dirtyCount}
              validationErrorCount={editState.validationErrorCount}
              errorSummaryText={editState.firstValidationErrorMessage}
              productCatalogId={activeHeroEntry.productCatalogId}
              prompt={editState.generatedPrompt}
              copiedPrompt={editState.copiedPrompt}
              onDiscard={editState.handleDiscardEdits}
              onCopyPrompt={editState.handleCopyPrompt}
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
