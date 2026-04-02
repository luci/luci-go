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

import {
  Close as CloseIcon,
  Edit as EditIcon,
  InfoOutlined as InfoOutlinedIcon,
  Share as ShareIcon,
} from '@mui/icons-material';
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
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useBlocker, useNavigate, useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  AddWidgetModal,
  BreakdownTableWidget,
  ChartWidget,
  DashboardDialog,
  DashboardTimeRangeSelector,
  DeleteDashboardDialog,
  FilterEditor,
  MarkdownWidget,
  RequireLogin,
  ShareDashboardDialog,
  WidgetContainer,
  useTopBarConfig,
} from '@/crystal_ball/components';
import {
  DATA_SPEC_ID,
  GLOBAL_TIME_RANGE_COLUMN,
  GLOBAL_TIME_RANGE_FILTER_ID,
  MAX_PAGE_SIZE,
} from '@/crystal_ball/constants';
import {
  getDashboardStateQueryKey,
  useDeleteDashboardState,
  useGetDashboardState,
  useUpdateDashboardState,
} from '@/crystal_ball/hooks/use_dashboard_state_api';
import { useListMeasurementFilterColumns } from '@/crystal_ball/hooks/use_measurement_filter_api';
import { CRYSTAL_BALL_ROUTES } from '@/crystal_ball/routes';
import { WidgetType } from '@/crystal_ball/types';
import {
  formatApiError,
  isStringArray,
  sanitizeChartWidget,
} from '@/crystal_ball/utils';
import {
  BreakdownTableConfig_BreakdownAggregation,
  DashboardState,
  DeleteDashboardStateRequest,
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  perfChartWidget_ChartTypeFromJSON,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
  UpdateDashboardStateRequest,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

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

function getSafeChartType(chartType: unknown): PerfChartWidget_ChartType {
  if (
    (typeof chartType === 'string' || typeof chartType === 'number') &&
    chartType in PerfChartWidget_ChartType
  ) {
    return perfChartWidget_ChartTypeFromJSON(chartType);
  }
  return PerfChartWidget_ChartType.CHART_TYPE_UNSPECIFIED;
}

function getWidgetType(widget: PerfWidget): WidgetType {
  if (widget.markdown) return WidgetType.MARKDOWN;
  if (widget.chart) {
    const chartType = getSafeChartType(widget.chart.chartType);
    if (chartType === PerfChartWidget_ChartType.BREAKDOWN_TABLE) {
      return WidgetType.CHART_BREAKDOWN_TABLE;
    }
    return WidgetType.CHART_MULTI_METRIC;
  }
  throw new Error(`Unknown widget type: ${JSON.stringify(widget)}`);
}

interface WidgetContextProps {
  globalFilters: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns: boolean;
  dataSpecs: { [key: string]: PerfDataSpec } | undefined;
  dashboardId: string | undefined;
}

const renderChartWidget = (
  Component: React.ComponentType<{
    widget: PerfChartWidget;
    dashboardName: string;
    widgetId: string;
    globalFilters?: readonly PerfFilter[];
    filterColumns: readonly MeasurementFilterColumn[];
    isLoadingFilterColumns?: boolean;
    dataSpecs?: { [key: string]: PerfDataSpec };
    onUpdate: (updatedWidget: PerfChartWidget) => void;
  }>,
  widget: PerfWidget,
  context: WidgetContextProps,
  onUpdate: (updatedWidget: PerfWidget) => void,
) => (
  <Component
    widget={widget.chart!}
    dashboardName={`dashboardStates/${context.dashboardId}`}
    widgetId={widget.id}
    globalFilters={context.globalFilters}
    filterColumns={context.filterColumns}
    isLoadingFilterColumns={context.isLoadingFilterColumns}
    dataSpecs={context.dataSpecs}
    onUpdate={(updatedChart: PerfChartWidget) =>
      onUpdate({ ...widget, chart: updatedChart })
    }
  />
);

const WIDGET_RENDERERS: Record<
  WidgetType,
  (
    widget: PerfWidget,
    context: WidgetContextProps,
    onUpdate: (updatedWidget: PerfWidget) => void,
  ) => React.ReactNode
> = {
  [WidgetType.MARKDOWN]: (widget, _context, onUpdate) => (
    <MarkdownWidget widget={widget} onUpdate={onUpdate} />
  ),
  [WidgetType.CHART_BREAKDOWN_TABLE]: (widget, context, onUpdate) =>
    renderChartWidget(BreakdownTableWidget, widget, context, onUpdate),
  [WidgetType.CHART_MULTI_METRIC]: (widget, context, onUpdate) =>
    renderChartWidget(ChartWidget, widget, context, onUpdate),
  [WidgetType.CHART_REGRESSION_METRIC]: () => null,
  [WidgetType.CHART_INVOCATION_DISTRIBUTION]: () => null,
};

const WIDGET_CREATORS: Record<WidgetType, () => Partial<PerfWidget>> = {
  [WidgetType.MARKDOWN]: () => ({
    displayName: 'New Markdown Widget',
    markdown: { content: 'This is a new markdown widget.' },
  }),
  [WidgetType.CHART_MULTI_METRIC]: () => ({
    displayName: 'New Chart Widget',
    chart: {
      dataSpecId: DATA_SPEC_ID,
      displayName: 'New Chart Widget',
      chartType: PerfChartWidget_ChartType.MULTI_METRIC_CHART,
      effectiveChartType: PerfChartWidget_ChartType.MULTI_METRIC_CHART,
      series: [],
      filters: [],
      xAxis: undefined,
      leftYAxis: undefined,
      rightYAxis: undefined,
      seriesSplit: undefined,
      invocationDistributionConfig: undefined,
    },
  }),
  [WidgetType.CHART_BREAKDOWN_TABLE]: () => ({
    displayName: 'New Breakdown Table',
    chart: {
      dataSpecId: DATA_SPEC_ID,
      displayName: 'New Breakdown Table',
      chartType: PerfChartWidget_ChartType.BREAKDOWN_TABLE,
      effectiveChartType: PerfChartWidget_ChartType.BREAKDOWN_TABLE,
      series: [],
      filters: [],
      xAxis: undefined,
      leftYAxis: undefined,
      rightYAxis: undefined,
      seriesSplit: undefined,
      invocationDistributionConfig: undefined,
      breakdownTableWidgetChartConfig: {
        aggregations: [
          BreakdownTableConfig_BreakdownAggregation.COUNT,
          BreakdownTableConfig_BreakdownAggregation.MIN,
          BreakdownTableConfig_BreakdownAggregation.MAX,
        ],
      },
    },
  }),
  [WidgetType.CHART_REGRESSION_METRIC]: () => ({}),
  [WidgetType.CHART_INVOCATION_DISTRIBUTION]: () => ({}),
};

/**
 * A customizable dashboard page that renders a dynamic collection of widgets.
 */
export function DashboardPage() {
  const { dashboardId } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [addWidgetModalOpen, setAddWidgetModalOpen] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);

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

  const { data: filterColumnsResponse, isLoading: isLoadingFilterColumns } =
    useListMeasurementFilterColumns(
      {
        parent: `dashboardStates/${dashboardId}/dataSpecs/${DATA_SPEC_ID}`,
        pageSize: MAX_PAGE_SIZE,
        pageToken: '',
      },
      {
        enabled: !!dashboardId,
      },
    );

  const filterColumns = useMemo(
    () => filterColumnsResponse?.measurementFilterColumns ?? [],
    [filterColumnsResponse],
  );

  const globalFilterColumns = useMemo(
    () =>
      filterColumns.filter(
        (c) =>
          c.column !== GLOBAL_TIME_RANGE_COLUMN &&
          (c.applicableScopes?.includes(
            MeasurementFilterColumn_FilterScope.GLOBAL,
          ) ||
            (isStringArray(c.applicableScopes) &&
              c.applicableScopes.includes('GLOBAL'))),
      ),
    [filterColumns],
  );

  const [toastState, setToastState] = useState({ message: '', isError: false });

  const handleCloseToast = () => {
    setToastState((prev) => ({ ...prev, message: '' }));
  };

  const showSuccessToast = useCallback((message: string) => {
    setToastState({ message, isError: false });
  }, []);

  const showErrorToast = useCallback((e: unknown, defaultMessage: string) => {
    setToastState({
      message: formatApiError(e, defaultMessage),
      isError: true,
    });
  }, []);

  const [localDashboardState, setLocalDashboardState] =
    useState<DashboardState | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  const hasUnsavedChanges = useMemo(() => {
    return (
      localDashboardState?.displayName !== dashboardState?.displayName ||
      localDashboardState?.description !== dashboardState?.description ||
      !deepEqual(
        localDashboardState?.dashboardContent?.widgets,
        dashboardState?.dashboardContent?.widgets,
      ) ||
      !deepEqual(
        localDashboardState?.dashboardContent?.globalFilters,
        dashboardState?.dashboardContent?.globalFilters,
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
      await deleteDashboard(
        DeleteDashboardStateRequest.fromPartial({ name: dashboardState.name }),
      );
      showSuccessToast('Dashboard deleted successfully');
      setDeleteDialogOpen(false);
      queryClient.removeQueries({
        queryKey: getDashboardStateQueryKey(dashboardState.name),
      });
      navigate(CRYSTAL_BALL_ROUTES.LANDING, { replace: true });
    } catch (e) {
      showErrorToast(e, 'Failed to delete dashboard');
      setDeleteDialogOpen(false);
    }
  };

  const handleUpdatePublicAccess = async (isPublic: boolean) => {
    if (!dashboardState?.name) return;
    try {
      const response = await updateDashboard(
        UpdateDashboardStateRequest.fromPartial({
          dashboardState: DashboardState.fromPartial({
            name: dashboardState.name,
            isPublic: isPublic,
          }),
          updateMask: ['isPublic'],
        }),
      );
      if (response.response) {
        setLocalDashboardState((prev) => {
          if (!prev) return response.response!;
          return {
            ...response.response!,
            displayName: prev.displayName,
            description: prev.description,
            dashboardContent: prev.dashboardContent,
          };
        });
        showSuccessToast(`Dashboard made ${isPublic ? 'public' : 'private'}`);
        setShareDialogOpen(false);
        refetch();
      }
    } catch (e) {
      showErrorToast(e, 'Failed to update public access');
    }
  };

  const handleSaveToApi = useCallback(async () => {
    if (!localDashboardState || !localDashboardState.name) return;
    setIsSaving(true);
    try {
      const sanitizedWidgets =
        localDashboardState.dashboardContent?.widgets?.map((w) => {
          if (w.chart) {
            return PerfWidget.fromPartial({
              ...w,
              chart: sanitizeChartWidget(w.chart),
            });
          }
          return PerfWidget.fromPartial(w);
        }) ?? [];

      const payload = UpdateDashboardStateRequest.fromPartial({
        dashboardState: {
          ...localDashboardState,
          dashboardContent: {
            ...localDashboardState.dashboardContent,
            widgets: sanitizedWidgets,
            dataSpecs: localDashboardState.dashboardContent?.dataSpecs ?? {},
            globalFilters:
              localDashboardState.dashboardContent?.globalFilters ?? [],
          },
        },
        updateMask: [
          'displayName',
          'description',
          'dashboardContent.widgets',
          'dashboardContent.globalFilters',
        ],
      });

      const response = await updateDashboard(payload);

      if (response.response) {
        setLocalDashboardState(response.response);
        queryClient.setQueryData(
          getDashboardStateQueryKey(response.response.name),
          response.response,
        );
      }
      showSuccessToast('Dashboard saved successfully');
      await refetch();
    } catch (e) {
      showErrorToast(e, 'Failed to save dashboard');
    } finally {
      setIsSaving(false);
    }
  }, [
    localDashboardState,
    updateDashboard,
    refetch,
    queryClient,
    showSuccessToast,
    showErrorToast,
  ]);

  const handleAddWidget = useCallback((widgetType: WidgetType) => {
    setLocalDashboardState((prev: DashboardState | null) => {
      if (!prev) return null;

      const creator = WIDGET_CREATORS[widgetType];
      const widgetPartial = creator ? creator() : {};

      const newWidget = PerfWidget.fromPartial({
        id: `widget-${crypto.randomUUID()}`,
        ...widgetPartial,
      });

      return {
        ...prev,
        dashboardContent: {
          ...prev.dashboardContent,
          widgets: [...(prev.dashboardContent?.widgets ?? []), newWidget],
          dataSpecs: prev.dashboardContent?.dataSpecs ?? {},
          globalFilters: prev.dashboardContent?.globalFilters ?? [],
        },
      };
    });
    setAddWidgetModalOpen(false);
  }, []);

  const handleUpdateWidget = useCallback(
    (index: number, updatedWidget: PerfWidget) => {
      setLocalDashboardState((prev: DashboardState | null) => {
        if (!prev || !prev.dashboardContent?.widgets) return null;
        const newWidgets = [...prev.dashboardContent.widgets];
        newWidgets[index] = updatedWidget;
        return {
          ...prev,
          dashboardContent: {
            ...prev.dashboardContent,
            widgets: newWidgets,
            dataSpecs: prev.dashboardContent?.dataSpecs ?? {},
            globalFilters: prev.dashboardContent?.globalFilters ?? [],
          },
        };
      });
    },
    [],
  );

  const handleDeleteWidget = useCallback((index: number) => {
    setLocalDashboardState((prev: DashboardState | null) => {
      if (!prev || !prev.dashboardContent?.widgets) return prev || null;
      const newWidgets = [...prev.dashboardContent.widgets];
      newWidgets.splice(index, 1);
      return {
        ...prev,
        dashboardContent: {
          ...prev.dashboardContent,
          widgets: newWidgets,
          dataSpecs: prev.dashboardContent?.dataSpecs ?? {},
          globalFilters: prev.dashboardContent?.globalFilters ?? [],
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
            dataSpecs: prev.dashboardContent?.dataSpecs ?? {},
            globalFilters: prev.dashboardContent?.globalFilters ?? [],
          },
        };
      });
    },
    [],
  );

  const handleUpdateGlobalFilters = useCallback(
    (updatedFilters: PerfFilter[]) => {
      setLocalDashboardState((prev: DashboardState | null) => {
        if (!prev) return null;

        const conservedPrimaryFilters = (
          prev.dashboardContent?.globalFilters ?? []
        ).filter((f) => f.id === GLOBAL_TIME_RANGE_FILTER_ID);

        return {
          ...prev,
          dashboardContent: {
            ...prev.dashboardContent,
            widgets: prev.dashboardContent?.widgets ?? [],
            dataSpecs: prev.dashboardContent?.dataSpecs ?? {},
            globalFilters: [...conservedPrimaryFilters, ...updatedFilters],
          },
        };
      });
    },
    [],
  );

  const topBarAction = useMemo(
    () => (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        {localDashboardState && (
          <DashboardTimeRangeSelector
            dashboardState={localDashboardState}
            onApply={setLocalDashboardState}
          />
        )}
        {hasUnsavedChanges && !isUpdating && !isSaving && (
          <Button
            variant="contained"
            color="error"
            onClick={() => setLocalDashboardState(dashboardState ?? null)}
            disabled={isUpdating || isLoading || isSaving}
          >
            Undo changes
          </Button>
        )}
        <Button
          variant="contained"
          onClick={handleSaveToApi}
          disabled={!hasUnsavedChanges || isUpdating || isLoading || isSaving}
        >
          {isUpdating || isSaving ? 'Saving...' : 'Save'}
        </Button>
        <IconButton
          onClick={() => setShareDialogOpen(true)}
          aria-label="Share dashboard"
          size="small"
        >
          <ShareIcon fontSize="small" />
        </IconButton>
      </Box>
    ),
    [
      hasUnsavedChanges,
      isUpdating,
      isLoading,
      handleSaveToApi,
      localDashboardState,
      dashboardState,
      isSaving,
    ],
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

  const subHeader = useMemo(() => {
    if (!isLoadingFilterColumns && globalFilterColumns.length === 0)
      return null;

    const activeGlobalFilters = (
      localDashboardState?.dashboardContent?.globalFilters ?? []
    ).filter((f) => f.id !== GLOBAL_TIME_RANGE_FILTER_ID);

    return (
      <Box
        sx={{
          bgcolor: (theme) => theme.palette.action.hover,
          borderBottom: '1px solid',
          borderColor: 'divider',
        }}
      >
        <FilterEditor
          title="Global Filters"
          filters={activeGlobalFilters}
          onUpdateFilters={handleUpdateGlobalFilters}
          dataSpecId={DATA_SPEC_ID}
          availableColumns={globalFilterColumns}
          isLoadingColumns={isLoadingFilterColumns}
        />
      </Box>
    );
  }, [
    isLoadingFilterColumns,
    globalFilterColumns,
    localDashboardState,
    handleUpdateGlobalFilters,
  ]);

  useTopBarConfig(topBarTitle, topBarAction, topBarMenuItems, subHeader);

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
      {localDashboardState &&
        (!localDashboardState.dashboardContent ||
          !localDashboardState.dashboardContent.widgets ||
          localDashboardState.dashboardContent.widgets.length === 0) && (
          <EmptyDashboardState onAdd={() => setAddWidgetModalOpen(true)} />
        )}

      {localDashboardState?.dashboardContent?.widgets &&
        localDashboardState.dashboardContent.widgets?.map(
          (widget: PerfWidget, index: number) => {
            if (!widget) return null;
            const widgetType = getWidgetType(widget);

            return (
              <WidgetContainer
                key={widget.id ?? `widget-${index}`}
                title={widget.displayName ?? 'Widget'}
                disablePadding={widgetType === WidgetType.CHART_BREAKDOWN_TABLE}
                onMoveUp={
                  index > 0 ? () => handleMoveWidget(index, 'UP') : undefined
                }
                onMoveDown={
                  index <
                  (localDashboardState.dashboardContent?.widgets?.length || 0) -
                    1
                    ? () => handleMoveWidget(index, 'DOWN')
                    : undefined
                }
                onDelete={() => handleDeleteWidget(index)}
                onTitleChange={(newTitle) =>
                  handleUpdateWidget(index, {
                    ...widget,
                    displayName: newTitle,
                  })
                }
              >
                {(() => {
                  const widgetType = getWidgetType(widget);
                  if (!WIDGET_RENDERERS[widgetType]) return null;
                  return (
                    <RecoverableErrorBoundary key={index}>
                      {WIDGET_RENDERERS[widgetType](
                        widget,
                        {
                          globalFilters:
                            localDashboardState?.dashboardContent
                              ?.globalFilters ?? [],
                          filterColumns,
                          isLoadingFilterColumns,
                          dataSpecs:
                            localDashboardState?.dashboardContent?.dataSpecs,
                          dashboardId,
                        },
                        (updatedWidget) =>
                          handleUpdateWidget(index, updatedWidget),
                      )}
                    </RecoverableErrorBoundary>
                  );
                })()}
              </WidgetContainer>
            );
          },
        )}

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
        open={Boolean(toastState.message)}
        autoHideDuration={toastState.isError ? undefined : 4000}
        onClose={handleCloseToast}
        message={toastState.message}
        slotProps={{
          content: {
            sx: toastState.isError
              ? { bgcolor: 'error.main', color: 'error.contrastText' }
              : {},
          },
        }}
        action={
          toastState.isError ? (
            <>
              <Button
                color="inherit"
                size="small"
                onClick={() =>
                  navigator.clipboard.writeText(toastState.message)
                }
              >
                COPY
              </Button>
              <IconButton
                size="small"
                aria-label="close"
                color="inherit"
                onClick={handleCloseToast}
              >
                <CloseIcon fontSize="small" />
              </IconButton>
            </>
          ) : undefined
        }
      />
      <DeleteDashboardDialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
        onConfirm={handleDelete}
        isDeleting={isDeleting}
        dashboardState={dashboardState ?? null}
      />
      <ShareDashboardDialog
        open={shareDialogOpen}
        onClose={() => setShareDialogOpen(false)}
        dashboardState={localDashboardState}
        onApplyPermissions={handleUpdatePublicAccess}
        isPending={isUpdating}
      />
    </Box>
  );
}

/**
 * Component export for Dashboard Page.
 */
export function Component() {
  return (
    <RequireLogin>
      <DashboardPage />
    </RequireLogin>
  );
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
