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
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  MenuItem,
  Popover,
  Snackbar,
  Typography,
} from '@mui/material';
import { useQueryClient } from '@tanstack/react-query';
import { deepEqual } from 'fast-equals';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useBlocker, useNavigate, useParams } from 'react-router';

import { TimeRangeSelector } from '@/common/components/time_range_selector';
import {
  AddWidgetModal,
  ChartWidget,
  MarkdownWidget,
  WidgetContainer,
} from '@/crystal_ball/components';
import { DashboardDialog } from '@/crystal_ball/components/dashboard_dialog';
import { DeleteDashboardDialog } from '@/crystal_ball/components/dashboard_dialog/delete_dashboard_dialog';
import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import { DATA_SPEC_ID } from '@/crystal_ball/constants';
import {
  getDashboardStateQueryKey,
  useDeleteDashboardState,
  useGetDashboardState,
  useUpdateDashboardState,
} from '@/crystal_ball/hooks/use_dashboard_state_api';
import { DashboardState, PerfWidget, WidgetType } from '@/crystal_ball/types';
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
 * A customizable dashboard page that renders a dynamic collection of widgets.
 */
export function DashboardPage() {
  const { dashboardId } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [addWidgetModalOpen, setAddWidgetModalOpen] = useState(false);

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
      localDashboardState?.description !== dashboardState?.description ||
      !deepEqual(
        localDashboardState?.dashboardContent?.widgets,
        dashboardState?.dashboardContent?.widgets,
      )
    );
  }, [localDashboardState, dashboardState]);

  useEffect(() => {
    if (dashboardState && (!localDashboardState || !hasUnsavedChanges)) {
      setLocalDashboardState(dashboardState);
    }
  }, [dashboardState, localDashboardState, hasUnsavedChanges]);

  useEffect(() => {
    if (!hasUnsavedChanges) return;

    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, [hasUnsavedChanges]);

  const blocker = useBlocker(hasUnsavedChanges);

  useEffect(() => {
    if (blocker.state === 'blocked') {
      if (
        window.confirm(
          'You have unsaved changes. Are you sure you want to leave?',
        )
      ) {
        blocker.proceed();
      } else {
        blocker.reset();
      }
    }
  }, [blocker]);

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
        updateMask: {
          paths: ['displayName', 'description', 'dashboardContent.widgets'],
        },
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

  const handleAddWidget = useCallback((widgetType: WidgetType) => {
    setLocalDashboardState((prev) => {
      if (!prev) return prev;
      const newWidget: PerfWidget = {
        id: `widget-${crypto.randomUUID()}`,
        displayName: 'New Widget',
      };
      if (widgetType === WidgetType.MARKDOWN) {
        newWidget.markdown = { content: 'This is a new markdown widget.' };
      } else if (widgetType === WidgetType.CHART_MULTI_METRIC) {
        newWidget.chart = {
          dataSpecId: DATA_SPEC_ID,
          displayName: 'New Chart Widget',
          chartType: 'MULTI_METRIC_CHART',
          series: [],
          filters: [],
        };
      }
      return {
        ...prev,
        dashboardContent: {
          ...prev.dashboardContent,
          widgets: [...(prev.dashboardContent?.widgets || []), newWidget],
        },
      };
    });
    setAddWidgetModalOpen(false);
  }, []);

  const handleUpdateWidget = useCallback(
    (index: number, updatedWidget: PerfWidget) => {
      setLocalDashboardState((prev) => {
        if (!prev || !prev.dashboardContent?.widgets) return prev;
        const newWidgets = [...prev.dashboardContent.widgets];
        newWidgets[index] = updatedWidget;
        return {
          ...prev,
          dashboardContent: {
            ...prev.dashboardContent,
            widgets: newWidgets,
          },
        };
      });
    },
    [],
  );

  const handleDeleteWidget = useCallback((index: number) => {
    setLocalDashboardState((prev) => {
      if (!prev || !prev.dashboardContent?.widgets) return prev;
      const newWidgets = [...prev.dashboardContent.widgets];
      newWidgets.splice(index, 1);
      return {
        ...prev,
        dashboardContent: {
          ...prev.dashboardContent,
          widgets: newWidgets,
        },
      };
    });
  }, []);

  const handleMoveWidget = useCallback(
    (index: number, direction: 'UP' | 'DOWN') => {
      setLocalDashboardState((prev) => {
        if (!prev || !prev.dashboardContent?.widgets) return prev;
        const newWidgets = [...prev.dashboardContent.widgets];
        const targetIndex = direction === 'UP' ? index - 1 : index + 1;
        if (targetIndex < 0 || targetIndex >= newWidgets.length) return prev;

        const temp = newWidgets[index];
        newWidgets[index] = newWidgets[targetIndex];
        newWidgets[targetIndex] = temp;

        return {
          ...prev,
          dashboardContent: {
            ...prev.dashboardContent,
            widgets: newWidgets,
          },
        };
      });
    },
    [],
  );

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
    <Box
      sx={{
        p: 3,
        display: 'flex',
        flexDirection: 'column',
        gap: 2,
        minWidth: 0,
      }}
    >
      {(!localDashboardState?.dashboardContent?.widgets ||
        localDashboardState.dashboardContent.widgets.length === 0) && (
        <EmptyDashboardState onAdd={() => setAddWidgetModalOpen(true)} />
      )}

      {localDashboardState?.dashboardContent?.widgets?.map((widget, index) => {
        if (!widget) return null;

        return (
          <WidgetContainer
            key={widget.id || `widget-${index}`}
            title={widget.displayName || 'Widget'}
            onMoveUp={
              index > 0 ? () => handleMoveWidget(index, 'UP') : undefined
            }
            onMoveDown={
              index <
              (localDashboardState.dashboardContent.widgets?.length || 0) - 1
                ? () => handleMoveWidget(index, 'DOWN')
                : undefined
            }
            onDelete={() => handleDeleteWidget(index)}
            onTitleChange={(newTitle) =>
              handleUpdateWidget(index, { ...widget, displayName: newTitle })
            }
          >
            {widget.markdown && (
              <MarkdownWidget
                widget={widget}
                onUpdate={(updatedWidget) =>
                  handleUpdateWidget(index, updatedWidget)
                }
              />
            )}
            {widget.chart && (
              <ChartWidget
                widget={widget.chart}
                onUpdate={(updatedChartWidget) =>
                  handleUpdateWidget(index, {
                    ...widget,
                    chart: updatedChartWidget,
                  })
                }
              />
            )}
          </WidgetContainer>
        );
      })}

      {!!localDashboardState?.dashboardContent?.widgets?.length && (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
          <Button
            variant="contained"
            disableElevation
            onClick={() => setAddWidgetModalOpen(true)}
          >
            Add Widget
          </Button>
        </Box>
      )}

      <AddWidgetModal
        open={addWidgetModalOpen}
        onClose={() => setAddWidgetModalOpen(false)}
        onAdd={handleAddWidget}
      />

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

function EmptyDashboardState({ onAdd }: { onAdd: () => void }) {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        p: 6,
        my: 4,
        border: '1px dashed',
        borderColor: 'divider',
        borderRadius: 2,
        bgcolor: 'background.default',
      }}
    >
      <Typography variant="h6" color="text.secondary" gutterBottom>
        This dashboard is empty
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Add a widget to start building your custom view.
      </Typography>
      <Button variant="contained" disableElevation onClick={onAdd}>
        Add Widget
      </Button>
    </Box>
  );
}
