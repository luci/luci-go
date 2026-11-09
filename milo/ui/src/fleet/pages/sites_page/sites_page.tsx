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

import AddIcon from '@mui/icons-material/Add';
import ClearIcon from '@mui/icons-material/Clear';
import SearchIcon from '@mui/icons-material/Search';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  InputAdornment,
  TextField,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFeatureFlag } from '@/common/feature_flags';
import { AdminAccessRequiredDialog } from '@/fleet/components/actions/shared/admin_access_required_dialog';
import { useAdminTaskPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { enablePhysicalSites } from '@/fleet/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { PageNotFoundPage } from '@/fleet/pages/not_found_page';
import {
  actionRowSx,
  iconButtonSx,
  pageHeaderSx,
  pillInputSx,
  surfaceCardSx,
} from '@/fleet/theme/recipes';
import { borders, fleetColors, iconSizes } from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { DeleteSiteDialog } from './delete_site_dialog';
import { SiteCard } from './site_card';
import { SiteFormDialog } from './site_form_dialog';
import { usePhysicalSites } from './use_physical_sites';

const headerWrapSx = {
  ...actionRowSx,
  justifyContent: 'space-between',
  flexWrap: 'wrap',
  gap: 1.5,
} as const;
const gridSx = {
  display: 'grid',
  gridTemplateColumns: {
    xs: '1fr',
    md: 'repeat(2, minmax(0, 1fr))',
    lg: 'repeat(3, minmax(0, 1fr))',
  },
  gap: 2.5,
} as const;
const emptyMatchSx = {
  ...surfaceCardSx,
  p: 4,
  textAlign: 'center',
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  gap: 1.5,
} as const;
const searchIconSx = {
  fontSize: iconSizes.inline,
  color: fleetColors.textTertiary,
} as const;
const filterAriaProps = { 'aria-label': 'Filter physical sites' } as const;
const ctrlIconSx = { fontSize: iconSizes.control } as const;

export const SitesPage = () => {
  const api = usePhysicalSites();
  const { sites, isLoading, isError, error } = api;
  const { createSiteMutation, updateSiteMutation, deleteSiteMutation } = api;
  const { hasPermission, fetchPermissions } = useAdminTaskPermission();
  const [searchQuery, setSearchQuery] = useState('');
  const [isFormOpen, setIsFormOpen] = useState(false);
  const [editingSite, setEditingSite] = useState<PhysicalSite | null>(null);
  const [deletingSite, setDeletingSite] = useState<PhysicalSite | null>(null);
  const [isAdminDialogOpen, setIsAdminDialogOpen] = useState(false);

  const q = searchQuery.trim().toLowerCase();
  const filteredSites = useMemo(() => {
    const sorted = [...sites].sort(
      (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
    );
    return q
      ? sorted.filter(
          (s) =>
            s.name.toLowerCase().includes(q) || s.id.toLowerCase().includes(q),
        )
      : sorted;
  }, [sites, q]);

  const runWithAdmin = async (action: () => void) => {
    if (hasPermission === true) return action();
    if (hasPermission === false) return setIsAdminDialogOpen(true);
    try {
      const res = await fetchPermissions();
      if (res.hasPermission) action();
      else setIsAdminDialogOpen(true);
    } catch {
      setIsAdminDialogOpen(true);
    }
  };

  const openForm = (site: PhysicalSite | null) =>
    void runWithAdmin(() => {
      createSiteMutation.reset();
      updateSiteMutation.reset();
      setEditingSite(site);
      setIsFormOpen(true);
    });

  const closeForm = () => {
    setIsFormOpen(false);
    setEditingSite(null);
  };

  const handleSaveSite = async (site: PhysicalSite) => {
    try {
      if (editingSite) await updateSiteMutation.mutateAsync(site);
      else await createSiteMutation.mutateAsync(site);
      closeForm();
    } catch {
      // Surfaced inside SiteFormDialog.
    }
  };

  const handleOpenDelete = (site: PhysicalSite) =>
    void runWithAdmin(() => {
      deleteSiteMutation.reset();
      setDeletingSite(site);
    });

  const handleConfirmDelete = async (id: string) => {
    try {
      await deleteSiteMutation.mutateAsync(id);
      setDeletingSite(null);
    } catch {
      // Surfaced inside DeleteSiteDialog.
    }
  };

  const searchSlotProps = {
    input: {
      sx: { ...pillInputSx, minWidth: 260 },
      startAdornment: (
        <InputAdornment position="start">
          <SearchIcon sx={searchIconSx} />
        </InputAdornment>
      ),
      endAdornment: searchQuery ? (
        <InputAdornment position="end">
          <IconButton
            size="small"
            aria-label="Clear site filter"
            onClick={() => setSearchQuery('')}
            sx={iconButtonSx}
          >
            <ClearIcon sx={ctrlIconSx} />
          </IconButton>
        </InputAdornment>
      ) : undefined,
    },
  };
  const countTxt = `${filteredSites.length} ${filteredSites.length === 1 ? 'site' : 'sites'}`;
  const isSaving = createSiteMutation.isPending || updateSiteMutation.isPending;
  const saveErr = editingSite
    ? updateSiteMutation.error
    : createSiteMutation.error;
  const formKey =
    editingSite?.id ?? (isFormOpen ? 'create-open' : 'create-closed');

  return (
    <Box
      component="main"
      sx={{ backgroundColor: fleetColors.canvas, minHeight: '100%' }}
    >
      <Box sx={{ ...pageHeaderSx, borderBottom: borders.default }}>
        <Box sx={headerWrapSx}>
          <Box sx={actionRowSx}>
            <Typography variant="h5" component="h1">
              Physical sites
            </Typography>
            {!isLoading && !isError && (
              <StatusPill tone="neutral" dot={false}>
                {countTxt}
              </StatusPill>
            )}
          </Box>
          <Box sx={actionRowSx}>
            <TextField
              size="small"
              placeholder="Filter by site name or code..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              inputProps={filterAriaProps}
              slotProps={searchSlotProps}
            />
            <Button
              variant="contained"
              startIcon={<AddIcon sx={ctrlIconSx} />}
              onClick={() => openForm(null)}
            >
              Add site
            </Button>
          </Box>
        </Box>
      </Box>

      <Box sx={{ p: 3, display: 'flex', flexDirection: 'column', gap: 2 }}>
        {isError ? (
          <Alert severity="error">
            {getErrorMessage(error, 'load physical sites')}
          </Alert>
        ) : isLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
            <CircularProgress size={28} aria-label="Loading physical sites" />
          </Box>
        ) : sites.length === 0 ? (
          <Box sx={emptyMatchSx}>
            <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
              No physical sites registered yet.
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Add a lab or datacenter to track device counts and health across
              platforms.
            </Typography>
          </Box>
        ) : filteredSites.length === 0 ? (
          <Box sx={emptyMatchSx}>
            <Typography variant="body1" color="text.secondary">
              {`No sites match "${searchQuery.trim()}".`}
            </Typography>
            <Button
              variant="toolbar"
              color="inherit"
              onClick={() => setSearchQuery('')}
            >
              Clear filter
            </Button>
          </Box>
        ) : (
          <Box sx={gridSx}>
            {filteredSites.map((site) => (
              <SiteCard
                key={site.id}
                site={site}
                onEdit={openForm}
                onDelete={handleOpenDelete}
              />
            ))}
          </Box>
        )}
      </Box>

      <SiteFormDialog
        key={formKey}
        open={isFormOpen}
        editingSite={editingSite}
        existingSites={sites}
        isSaving={isSaving}
        error={saveErr}
        onClose={closeForm}
        onSave={handleSaveSite}
      />
      <DeleteSiteDialog
        site={deletingSite}
        isDeleting={deleteSiteMutation.isPending}
        error={deleteSiteMutation.error}
        onClose={() => !deleteSiteMutation.isPending && setDeletingSite(null)}
        onConfirmDelete={handleConfirmDelete}
      />
      <AdminAccessRequiredDialog
        open={isAdminDialogOpen}
        onClose={() => setIsAdminDialogOpen(false)}
      />
    </Box>
  );
};

export function Component() {
  const showPhysicalSites = useFeatureFlag(enablePhysicalSites);
  if (!showPhysicalSites) return <PageNotFoundPage />;
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-sites">
      <FleetHelmet pageTitle="Sites" />
      <RecoverableErrorBoundary key="fleet-sites-page">
        <LoggedInBoundary>
          <SitesPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
