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
  Alert,
  Box,
  CircularProgress,
  ClickAwayListener,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AddSliceCard } from './add_slice_card';
import { HealthSliceDialog } from './health_slice_dialog';
import { HealthSliceItem } from './health_slice_item';
import { SlicesTrayDivider } from './slices_tray_divider';
import { useDefaultSlice, useHealthSlices } from './use_health_slices';
import { useSliceReorder } from './use_slice_reorder';

export interface HealthSlicesCardProps {
  /** The active AIP-160 filter expression from the global Filter Bar. */
  readonly globalFilter?: string;
  /** Callback fired when a slice is clicked to filter the entire dashboard. */
  readonly onSelectSlice?: (slice: HealthSlice) => void;
}

type SliceDialogState =
  | { mode: 'create' }
  | { mode: 'edit'; slice: HealthSlice }
  | null;

export const HealthSlicesCard = ({
  globalFilter = '',
  onSelectSlice,
}: HealthSlicesCardProps) => {
  const {
    slicesQuery,
    createSliceMutation,
    updateSliceMutation,
    deleteSliceMutation,
    reorderSlicesMutation,
  } = useHealthSlices();
  const serverSlices = useMemo(
    () => slicesQuery.data?.healthSlices ?? [],
    [slicesQuery.data],
  );

  const { defaultSliceId, setDefaultSliceId } = useDefaultSlice();
  const [dialogState, setDialogState] = useState<SliceDialogState>(null);
  const [isExpanded, setIsExpanded] = useState(false);

  // Auto-purge stale defaultSliceId if the slice no longer exists on the server
  useEffect(() => {
    if (!defaultSliceId || !slicesQuery.data?.healthSlices) return;
    const exists = slicesQuery.data.healthSlices.some(
      (s) => s.id === defaultSliceId,
    );
    if (!exists) {
      setDefaultSliceId(null);
    }
  }, [defaultSliceId, slicesQuery.data, setDefaultSliceId]);

  const gridRef = useRef<HTMLDivElement>(null);
  const trayRef = useRef<HTMLDivElement>(null);
  const [collapsedHeight, setCollapsedHeight] = useState<number | undefined>(
    undefined,
  );

  const {
    orderedSlices,
    setOrderedSlices,
    draggingSliceId,
    dragOverSliceId,
    setDragOverSliceId,
    isDraggingRef,
    handleDragStart,
    handleDragOver,
    handleDragEnter,
    handleDragLeave,
    handleDrop,
    handleDragEnd,
    handleDropAtEnd,
  } = useSliceReorder({
    serverSlices,
    onPersistOrder: (ids) => reorderSlicesMutation.mutate(ids),
    onExpandTray: () => setIsExpanded(true),
  });

  // Track collapsed height so opening absolute overlay tray does not collapse the in-flow container
  useEffect(() => {
    if (!isExpanded && trayRef.current) {
      const height = trayRef.current.offsetHeight;
      if (height > 0) {
        setCollapsedHeight(height);
      }
    }
  }, [isExpanded, orderedSlices.length]);

  useEffect(() => {
    if (
      typeof ResizeObserver === 'undefined' ||
      isExpanded ||
      !trayRef.current
    ) {
      return;
    }
    const el = trayRef.current;
    const observer = new ResizeObserver(() => {
      if (!isExpanded && el.offsetHeight > 0) {
        setCollapsedHeight(el.offsetHeight);
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [isExpanded]);

  // Auto-collapse tray when the user starts scrolling down the page
  useEffect(() => {
    if (!isExpanded) return;
    const handleScroll = () => {
      if (isDraggingRef.current) return;
      setIsExpanded(false);
    };
    window.addEventListener('scroll', handleScroll, { passive: true });
    return () => {
      window.removeEventListener('scroll', handleScroll);
    };
  }, [isExpanded, isDraggingRef]);

  const closeDialog = useCallback(() => {
    setDialogState(null);
    createSliceMutation.reset();
    updateSliceMutation.reset();
    deleteSliceMutation.reset();
  }, [createSliceMutation, updateSliceMutation, deleteSliceMutation]);

  const handleOpenAddSlice = useCallback(() => {
    setDialogState({ mode: 'create' });
  }, []);

  const handleOpenEditSlice = useCallback((sliceToEdit: HealthSlice) => {
    setDialogState({ mode: 'edit', slice: sliceToEdit });
  }, []);

  const isSaving =
    createSliceMutation.isPending ||
    updateSliceMutation.isPending ||
    deleteSliceMutation.isPending;

  const mutationError =
    createSliceMutation.error ??
    updateSliceMutation.error ??
    deleteSliceMutation.error;

  const handleSaveSlice = useCallback(
    (slice: HealthSlice) => {
      const isEditing = dialogState?.mode === 'edit';
      const mutation = isEditing ? updateSliceMutation : createSliceMutation;
      mutation.mutate(slice, { onSuccess: closeDialog });
    },
    [dialogState, createSliceMutation, updateSliceMutation, closeDialog],
  );

  const handleDeleteSlice = useCallback(
    (sliceIdToDelete: string) => {
      if (defaultSliceId === sliceIdToDelete) {
        setDefaultSliceId(null);
      }
      setOrderedSlices((prev) => prev.filter((s) => s.id !== sliceIdToDelete));
      deleteSliceMutation.mutate(sliceIdToDelete, { onSuccess: closeDialog });
    },
    [
      defaultSliceId,
      setDefaultSliceId,
      setOrderedSlices,
      deleteSliceMutation,
      closeDialog,
    ],
  );

  const handleSelectSlice = useCallback(
    (slice: HealthSlice) => {
      if (isDraggingRef.current) return;
      onSelectSlice?.(slice);
      setIsExpanded(false);
    },
    [onSelectSlice, isDraggingRef],
  );

  const handleToggleDefault = useCallback(
    (slice: HealthSlice) => {
      setDefaultSliceId(defaultSliceId === slice.id ? null : slice.id);
    },
    [defaultSliceId, setDefaultSliceId],
  );

  const handleClickAway = useCallback(() => {
    if (isDraggingRef.current) return;
    setIsExpanded(false);
  }, [isDraggingRef]);

  return (
    <Box data-testid="health-slices-card" sx={{ mb: 3, position: 'relative' }}>
      {reorderSlicesMutation.isError && (
        <Alert
          severity="error"
          onClose={() => reorderSlicesMutation.reset()}
          sx={{ mb: 2 }}
        >
          Failed to save slice order: {reorderSlicesMutation.error.message}
        </Alert>
      )}

      {/* Slices Row: Slices Query Loading / Error / Horizontal Ribbon */}
      {slicesQuery.isPending ? (
        <Box sx={{ display: 'flex', alignItems: 'center', py: 2 }}>
          <CircularProgress size={20} sx={{ mr: 1.5 }} />
          <Typography variant="body2" color="text.secondary">
            Loading health slices...
          </Typography>
        </Box>
      ) : slicesQuery.isError ? (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load health slices: {slicesQuery.error.message}
        </Alert>
      ) : (
        <Box
          sx={{
            position: 'relative',
            minHeight: isExpanded ? (collapsedHeight ?? 166) : undefined,
          }}
        >
          <ClickAwayListener
            onClickAway={isExpanded ? handleClickAway : () => {}}
          >
            <Box
              ref={trayRef}
              data-testid={isExpanded ? 'slices-overlay-tray' : undefined}
              sx={{
                position: isExpanded ? 'absolute' : 'relative',
                top: isExpanded ? '-13px' : '0px',
                left: isExpanded ? '-13px' : '0px',
                right: isExpanded ? '-13px' : '0px',
                zIndex: isExpanded ? 1200 : 1,
                bgcolor: isExpanded ? 'background.paper' : 'transparent',
                borderRadius: 2,
                p: isExpanded ? '12px' : '0px',
                border: isExpanded ? '1px solid' : 'none',
                borderColor: isExpanded ? 'divider' : 'transparent',
                boxShadow: isExpanded ? '0 16px 40px rgba(0,0,0,0.18)' : 'none',
                transition: 'box-shadow 0.15s ease, border-color 0.15s ease',
              }}
            >
              <Box
                ref={gridRef}
                sx={{
                  display: 'grid',
                  gridTemplateColumns: {
                    xs: '1fr',
                    sm: 'repeat(2, 1fr)',
                    md: 'repeat(4, 1fr)',
                  },
                  gap: 1.5,
                  maxHeight: isExpanded ? '65vh' : 'none',
                  overflowY: isExpanded ? 'auto' : 'visible',
                  overflowX: 'visible',
                  p: 0,
                  scrollbarWidth: 'thin',
                  '& > :nth-child(n+5)': isExpanded
                    ? undefined
                    : { display: 'none !important' },
                  '@media (max-width: 899.95px)': isExpanded
                    ? {}
                    : {
                        '& > :nth-child(n+3)': { display: 'none !important' },
                      },
                  '@media (max-width: 599.95px)': isExpanded
                    ? {}
                    : {
                        '& > :nth-child(n+2)': { display: 'none !important' },
                      },
                }}
              >
                {/* 1. Health Slices (Draggable) */}
                {orderedSlices.map((slice) => (
                  <HealthSliceItem
                    key={slice.id}
                    testId={`health-slice-item-${slice.id}`}
                    slice={slice}
                    globalFilter={globalFilter}
                    isSelected={globalFilter.trim() === slice.filter.trim()}
                    isDefault={defaultSliceId === slice.id}
                    isDragging={draggingSliceId === slice.id}
                    isDragOver={dragOverSliceId === slice.id}
                    isAnyDragging={Boolean(draggingSliceId)}
                    onSelect={handleSelectSlice}
                    onEdit={handleOpenEditSlice}
                    onDelete={(s) => handleDeleteSlice(s.id)}
                    onToggleDefault={handleToggleDefault}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragEnter={handleDragEnter}
                    onDragLeave={handleDragLeave}
                    onDrop={handleDrop}
                    onDragEnd={handleDragEnd}
                  />
                ))}

                {/* 2. Add Slice Button Card (Drop target) */}
                {(isExpanded || orderedSlices.length <= 3) && (
                  <AddSliceCard
                    isDragOver={dragOverSliceId === '__ADD_SLICE__'}
                    onClick={handleOpenAddSlice}
                    onDragOver={(e) => {
                      if (!draggingSliceId) return;
                      e.preventDefault();
                      e.dataTransfer.dropEffect = 'move';
                      setDragOverSliceId('__ADD_SLICE__');
                    }}
                    onDragLeave={() => {
                      if (dragOverSliceId === '__ADD_SLICE__')
                        setDragOverSliceId(null);
                    }}
                    onDrop={(e) => {
                      e.preventDefault();
                      handleDropAtEnd();
                    }}
                  />
                )}
              </Box>

              {/* Expand / Add Slice Divider Bar (when collapsed) */}
              <SlicesTrayDivider
                slicesCount={orderedSlices.length}
                isExpanded={isExpanded}
                onAddSlice={handleOpenAddSlice}
                onExpand={() => setIsExpanded(true)}
              />
            </Box>
          </ClickAwayListener>
        </Box>
      )}

      {dialogState && (
        <HealthSliceDialog
          open={true}
          editingSlice={dialogState.mode === 'edit' ? dialogState.slice : null}
          existingSlices={orderedSlices}
          onClose={closeDialog}
          onSave={handleSaveSlice}
          onDelete={handleDeleteSlice}
          isSaving={isSaving}
          error={mutationError}
        />
      )}
    </Box>
  );
};

export const HealthSlicesRibbon = HealthSlicesCard;
