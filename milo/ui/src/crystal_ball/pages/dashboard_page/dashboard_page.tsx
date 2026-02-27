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

import EditIcon from '@mui/icons-material/Edit';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
import Popover from '@mui/material/Popover';
import Snackbar from '@mui/material/Snackbar';
import Typography from '@mui/material/Typography';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';

import { TimeRangeSelector } from '@/common/components/time_range_selector';
import { DashboardDialog } from '@/crystal_ball/components/dashboard_dialog';
import { DeleteDashboardDialog } from '@/crystal_ball/components/dashboard_dialog/delete_dashboard_dialog';
import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import {
  getDashboardStateQueryKey,
  useDeleteDashboardState,
  useGetDashboardState,
  useUpdateDashboardState,
} from '@/crystal_ball/hooks/use_dashboard_state_api';
import { DashboardState } from '@/crystal_ball/types';
import { formatApiError } from '@/crystal_ball/utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

/**
 * Component to display and edit the dashboard title and description in the TopBar.
 */
function DashboardTitleBar({
  dashboardState,
  onApply,
}: {
  dashboardState: DashboardState;
  onApply: (state: DashboardState) => void;
}) {
  const [isEditing, setIsEditing] = useState(false);
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);

  const handleInfoClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };
  const handleInfoClose = () => {
    setAnchorEl(null);
  };
  const openInfo = Boolean(anchorEl);

  return (
    <>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          '& .edit-icon-btn': {
            opacity: 0.3,
            transition: 'opacity 0.2s',
          },
          '&:hover .edit-icon-btn, &:focus-within .edit-icon-btn': {
            opacity: 1,
          },
          '& .edit-icon-btn:focus-visible': {
            opacity: 1,
            outline: '2px solid',
            outlineOffset: '2px',
          },
        }}
      >
        <Typography variant="h6">{dashboardState.displayName}</Typography>
        {dashboardState.description && (
          <>
            <IconButton
              size="small"
              onClick={handleInfoClick}
              aria-label="View dashboard description"
            >
              <InfoOutlinedIcon fontSize="small" />
            </IconButton>
            <Popover
              open={openInfo}
              anchorEl={anchorEl}
              onClose={handleInfoClose}
              anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
              transformOrigin={{ vertical: 'top', horizontal: 'center' }}
            >
              <Box sx={{ p: 2, maxWidth: 400 }}>
                <Typography variant="body2">
                  {dashboardState.description}
                </Typography>
              </Box>
            </Popover>
          </>
        )}
        <IconButton
          size="small"
          onClick={() => setIsEditing(true)}
          className="edit-icon-btn"
          aria-label="Edit dashboard title and description"
        >
          <EditIcon fontSize="small" />
        </IconButton>
      </Box>
      <DashboardDialog
        open={isEditing}
        onClose={() => setIsEditing(false)}
        initialData={dashboardState}
        onSubmit={async (data) => {
          onApply({
            ...dashboardState,
            ...data,
          });
          setIsEditing(false);
        }}
        submitText="Apply"
      />
    </>
  );
}

/**
 * Empty dashboard page that displays the response of the get call.
 */
export function DashboardPage() {
  const { dashboardId } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  const {
    data: dashboardState,
    isLoading,
    error,
    refetch,
  } = useGetDashboardState(
    {
      name: `dashboardStates/${dashboardId}`,
    },
    {
      enabled: !!dashboardId,
    },
  );

  const [toastMessage, setToastMessage] = useState('');
  const [searchParams] = useSyncedSearchParams();
  const isFirstRender = useRef(true);

  const timeOption = searchParams.get('time_option');
  const startTimeParam = searchParams.get('start_time');
  const endTimeParam = searchParams.get('end_time');

  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false;
      return;
    }
    setToastMessage('Feature under construction');
  }, [timeOption, startTimeParam, endTimeParam]);

  const handleCloseToast = () => {
    setToastMessage('');
  };

  const [localDashboardState, setLocalDashboardState] =
    useState<DashboardState | null>(null);

  const hasUnsavedChanges = useMemo(() => {
    return (
      localDashboardState?.displayName !== dashboardState?.displayName ||
      localDashboardState?.description !== dashboardState?.description
    );
  }, [localDashboardState, dashboardState]);

  useEffect(() => {
    if (dashboardState && (!localDashboardState || !hasUnsavedChanges)) {
      setLocalDashboardState(dashboardState);
    }
  }, [dashboardState, localDashboardState, hasUnsavedChanges]);

  const { mutateAsync: updateDashboard, isPending: isUpdating } =
    useUpdateDashboardState();

  const { mutateAsync: deleteDashboard, isPending: isDeleting } =
    useDeleteDashboardState();

  const handleDelete = async () => {
    if (!dashboardState?.name) return;
    try {
      await deleteDashboard({ name: dashboardState.name });
      setToastMessage('Dashboard deleted successfully');
      setDeleteDialogOpen(false);
      queryClient.removeQueries({
        queryKey: getDashboardStateQueryKey(dashboardState.name),
      });
      navigate('/ui/labs/crystal-ball', { replace: true });
    } catch (e) {
      setToastMessage(formatApiError(e, 'Failed to delete dashboard'));
      setDeleteDialogOpen(false);
    }
  };

  const handleSaveToApi = useCallback(async () => {
    if (!localDashboardState || !localDashboardState.name) return;
    try {
      const response = await updateDashboard({
        dashboardState: localDashboardState,
        updateMask: { paths: ['displayName', 'description'] },
      });
      if (response.response) {
        setLocalDashboardState(response.response);
      }
      setToastMessage('Dashboard saved successfully');
      refetch();
    } catch (e) {
      setToastMessage(formatApiError(e, 'Failed to save dashboard'));
    }
  }, [localDashboardState, updateDashboard, refetch]);

  const topBarAction = useMemo(
    () => (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <TimeRangeSelector />
        <Button
          variant="contained"
          onClick={handleSaveToApi}
          disabled={!hasUnsavedChanges || isUpdating || isLoading}
        >
          {isUpdating ? 'Saving...' : 'Save'}
        </Button>
      </Box>
    ),
    [hasUnsavedChanges, isUpdating, isLoading, handleSaveToApi],
  );

  const topBarTitle = useMemo(() => {
    if (localDashboardState) {
      return (
        <DashboardTitleBar
          dashboardState={localDashboardState}
          onApply={setLocalDashboardState}
        />
      );
    }
    return 'Loading...';
  }, [localDashboardState]);

  const topBarMenuItems = useMemo(
    () => (
      <MenuItem
        onClick={() => {
          setDeleteDialogOpen(true);
        }}
        sx={{ color: 'error.main' }}
      >
        Delete Dashboard
      </MenuItem>
    ),
    [],
  );

  useTopBarConfig(topBarTitle, topBarAction, topBarMenuItems);

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">
          Failed to load dashboard: {formatApiError(error)}
        </Alert>
      </Box>
    );
  }

  if (!dashboardState) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="warning">Dashboard not found</Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3 }}>
      <pre style={{ whiteSpace: 'pre-wrap', wordWrap: 'break-word' }}>
        {JSON.stringify(dashboardState, null, 2)}
      </pre>
      <Snackbar
        open={Boolean(toastMessage)}
        autoHideDuration={4000}
        onClose={handleCloseToast}
        message={toastMessage}
      />
      <DeleteDashboardDialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
        onConfirm={handleDelete}
        isDeleting={isDeleting}
        dashboardState={dashboardState}
      />
    </Box>
  );
}

/**
 * Component export for Dashboard Page.
 */
export function Component() {
  return <DashboardPage />;
}
