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
import { useCallback, useMemo, useState } from 'react';

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

// Physical Sites directory page.
//
// Key design decisions:
// 1. Naming ("Physical Sites" at /ui/fleet/sites vs. "Physical Labs"):
//    Selected in a 6-candidate naming tournament to match FLOPS facility
//    terminology without colliding with /ui/fleet/labs experimental routes.
// 2. Primary identifier ("string id" building code vs. surrogate int64):
//    Sites use canonical building codes (e.g. "EM25", "MTV") as their primary
//    string ID (matching Device.id hostnames) paired with a human-readable
//    display name (e.g. "Palo Alto EM25").
export const SitesPage = () => {
  const {
    sites,
    isLoading,
    isError,
    error,
    createSiteMutation,
    deleteSiteMutation,
  } = usePhysicalSites();
  const { hasPermission: hasAdminPermission, fetchPermissions } =
    useAdminTaskPermission();
  const [searchQuery, setSearchQuery] = useState('');
  const [isFormOpen, setIsFormOpen] = useState(false);
  const [deletingSite, setDeletingSite] = useState<PhysicalSite | null>(null);
  const [isAdminAccessDialogOpen, setIsAdminAccessDialogOpen] = useState(false);

  const trimmedQuery = searchQuery.trim().toLowerCase();

  const sortedSites = useMemo(() => {
    return [...sites].sort(
      (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
    );
  }, [sites]);

  const filteredSites = useMemo(() => {
    if (!trimmedQuery) {
      return sortedSites;
    }
    return sortedSites.filter(
      (site) =>
        site.name.toLowerCase().includes(trimmedQuery) ||
        site.id.toLowerCase().includes(trimmedQuery),
    );
  }, [sortedSites, trimmedQuery]);

  const runWithAdminPermission = useCallback(
    async (action: () => void) => {
      if (hasAdminPermission === true) {
        action();
        return;
      }
      if (hasAdminPermission === false) {
        setIsAdminAccessDialogOpen(true);
        return;
      }
      try {
        const res = await fetchPermissions();
        if (res.hasPermission) {
          action();
        } else {
          setIsAdminAccessDialogOpen(true);
        }
      } catch {
        setIsAdminAccessDialogOpen(true);
      }
    },
    [fetchPermissions, hasAdminPermission],
  );

  const handleOpenCreate = useCallback(() => {
    void runWithAdminPermission(() => {
      createSiteMutation.reset();
      setIsFormOpen(true);
    });
  }, [createSiteMutation, runWithAdminPermission]);

  const handleCloseForm = useCallback(() => {
    setIsFormOpen(false);
  }, []);

  const handleSaveSite = useCallback(
    async (site: PhysicalSite) => {
      try {
        await createSiteMutation.mutateAsync(site);
        setIsFormOpen(false);
      } catch {
        // Error is surfaced inside SiteFormDialog via mutation state.
      }
    },
    [createSiteMutation],
  );

  const handleOpenDelete = useCallback(
    (site: PhysicalSite) => {
      void runWithAdminPermission(() => {
        deleteSiteMutation.reset();
        setDeletingSite(site);
      });
    },
    [deleteSiteMutation, runWithAdminPermission],
  );

  const handleCloseDelete = useCallback(() => {
    if (deleteSiteMutation.isPending) {
      return;
    }
    setDeletingSite(null);
  }, [deleteSiteMutation.isPending]);

  const handleConfirmDelete = useCallback(
    async (id: string) => {
      try {
        await deleteSiteMutation.mutateAsync(id);
        setDeletingSite(null);
      } catch {
        // Error is surfaced inside DeleteSiteDialog via mutation state.
      }
    },
    [deleteSiteMutation],
  );

  return (
    <Box
      component="main"
      sx={{
        backgroundColor: fleetColors.canvas,
        minHeight: '100%',
      }}
    >
      <Box
        sx={{
          ...pageHeaderSx,
          borderBottom: borders.default,
        }}
      >
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: 1.5,
          }}
        >
          <Box sx={actionRowSx}>
            <Typography variant="h5" component="h1">
              Physical sites
            </Typography>
            {!isLoading && !isError && (
              <StatusPill tone="neutral" dot={false}>
                {`${filteredSites.length} ${
                  filteredSites.length === 1 ? 'site' : 'sites'
                }`}
              </StatusPill>
            )}
          </Box>

          <Box sx={actionRowSx}>
            <TextField
              size="small"
              placeholder="Filter by site name or code..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              inputProps={{ 'aria-label': 'Filter physical sites' }}
              slotProps={{
                input: {
                  sx: { ...pillInputSx, minWidth: 260 },
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon
                        sx={{
                          fontSize: iconSizes.inline,
                          color: fleetColors.textTertiary,
                        }}
                      />
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
                        <ClearIcon sx={{ fontSize: iconSizes.control }} />
                      </IconButton>
                    </InputAdornment>
                  ) : undefined,
                },
              }}
            />
            <Button
              variant="toolbar"
              startIcon={<AddIcon sx={{ fontSize: iconSizes.control }} />}
              onClick={handleOpenCreate}
            >
              Add site
            </Button>
          </Box>
        </Box>
      </Box>

      <Box
        sx={{
          p: 3,
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
        }}
      >
        {isError ? (
          <Alert severity="error">
            {getErrorMessage(error, 'load physical sites')}
          </Alert>
        ) : isLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
            <CircularProgress size={28} aria-label="Loading physical sites" />
          </Box>
        ) : sites.length === 0 ? (
          <Box sx={{ ...surfaceCardSx, p: 4, textAlign: 'center' }}>
            <Typography variant="body1" color="text.secondary">
              No physical sites registered yet.
            </Typography>
          </Box>
        ) : filteredSites.length === 0 ? (
          <Box
            sx={{
              ...surfaceCardSx,
              p: 4,
              textAlign: 'center',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: 1.5,
            }}
          >
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
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
              gap: 2,
            }}
          >
            {filteredSites.map((site) => (
              <SiteCard key={site.id} site={site} onDelete={handleOpenDelete} />
            ))}
          </Box>
        )}
      </Box>

      <SiteFormDialog
        key={isFormOpen ? 'create-open' : 'create-closed'}
        open={isFormOpen}
        existingSites={sites}
        isSaving={createSiteMutation.isPending}
        error={createSiteMutation.error}
        onClose={handleCloseForm}
        onSave={handleSaveSite}
      />

      <DeleteSiteDialog
        site={deletingSite}
        isDeleting={deleteSiteMutation.isPending}
        error={deleteSiteMutation.error}
        onClose={handleCloseDelete}
        onConfirmDelete={handleConfirmDelete}
      />

      <AdminAccessRequiredDialog
        open={isAdminAccessDialogOpen}
        onClose={() => setIsAdminAccessDialogOpen(false)}
      />
    </Box>
  );
};

export function Component() {
  const showPhysicalSites = useFeatureFlag(enablePhysicalSites);
  if (!showPhysicalSites) {
    return <PageNotFoundPage />;
  }

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
