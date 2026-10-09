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

import { useCallback, useEffect, useRef, useState } from 'react';

import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface UseSliceReorderOptions {
  readonly serverSlices: readonly HealthSlice[];
  readonly onPersistOrder: (orderedIds: readonly string[]) => void;
  readonly onExpandTray?: () => void;
  readonly debounceMs?: number;
}

export const useSliceReorder = ({
  serverSlices,
  onPersistOrder,
  onExpandTray,
  debounceMs = 500,
}: UseSliceReorderOptions) => {
  const [localSlices, setLocalSlices] =
    useState<readonly HealthSlice[]>(serverSlices);
  const [draggingSliceId, setDraggingSliceId] = useState<string | null>(null);
  const [dragOverSliceId, setDragOverSliceId] = useState<string | null>(null);
  const isDraggingRef = useRef(false);
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingIdsRef = useRef<readonly string[] | null>(null);
  const onPersistOrderRef = useRef(onPersistOrder);
  onPersistOrderRef.current = onPersistOrder;

  // Sync with server slices when not actively dragging or pending debounce
  useEffect(() => {
    if (!isDraggingRef.current && !debounceTimerRef.current) {
      setLocalSlices(serverSlices);
    }
  }, [serverSlices]);

  // Flush pending reorder on unmount so navigating away doesn't discard order changes
  useEffect(
    () => () => {
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
        debounceTimerRef.current = null;
        if (pendingIdsRef.current) {
          onPersistOrderRef.current(pendingIdsRef.current);
          pendingIdsRef.current = null;
        }
      }
    },
    [],
  );

  const updateOrder = useCallback(
    (newSlices: readonly HealthSlice[]) => {
      setLocalSlices(newSlices);
      const newIds = newSlices.map((s) => s.id);
      pendingIdsRef.current = newIds;
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
      }
      debounceTimerRef.current = setTimeout(() => {
        debounceTimerRef.current = null;
        pendingIdsRef.current = null;
        onPersistOrderRef.current(newIds);
      }, debounceMs);
    },
    [debounceMs],
  );

  const handleReorder = useCallback(
    (sourceId: string, targetId: string) => {
      if (sourceId === targetId) return;
      const currentIds = localSlices.map((s) => s.id);
      const sourceIndex = currentIds.indexOf(sourceId);
      const targetIndex = currentIds.indexOf(targetId);
      if (sourceIndex === -1 || targetIndex === -1) return;

      const newSlices = [...localSlices];
      const [movedSlice] = newSlices.splice(sourceIndex, 1);
      newSlices.splice(targetIndex, 0, movedSlice);
      updateOrder(newSlices);
    },
    [localSlices, updateOrder],
  );

  const handleDragStart = useCallback(
    (slice: HealthSlice) => {
      isDraggingRef.current = true;
      setDraggingSliceId(slice.id);
      if (localSlices.length >= 5) {
        onExpandTray?.();
      }
    },
    [localSlices.length, onExpandTray],
  );

  const handleDragOver = useCallback(
    (slice: HealthSlice) => {
      if (draggingSliceId && draggingSliceId !== slice.id) {
        setDragOverSliceId(slice.id);
      }
    },
    [draggingSliceId],
  );

  const handleDragEnter = useCallback(
    (slice: HealthSlice) => {
      if (draggingSliceId && draggingSliceId !== slice.id) {
        setDragOverSliceId(slice.id);
      }
    },
    [draggingSliceId],
  );

  const handleDragLeave = useCallback(
    (slice: HealthSlice) => {
      if (dragOverSliceId === slice.id) {
        setDragOverSliceId(null);
      }
    },
    [dragOverSliceId],
  );

  const handleDrop = useCallback(
    (targetSlice: HealthSlice) => {
      if (draggingSliceId && draggingSliceId !== targetSlice.id) {
        handleReorder(draggingSliceId, targetSlice.id);
      }
      setDraggingSliceId(null);
      setDragOverSliceId(null);
      setTimeout(() => {
        isDraggingRef.current = false;
      }, 100);
    },
    [draggingSliceId, handleReorder],
  );

  const handleDragEnd = useCallback(() => {
    setDraggingSliceId(null);
    setDragOverSliceId(null);
    setTimeout(() => {
      isDraggingRef.current = false;
    }, 100);
  }, []);

  const handleDropAtEnd = useCallback(() => {
    if (!draggingSliceId) return;
    const currentIds = localSlices.map((s) => s.id);
    const sourceIndex = currentIds.indexOf(draggingSliceId);
    if (sourceIndex !== -1 && sourceIndex < currentIds.length - 1) {
      const newSlices = [...localSlices];
      const [movedSlice] = newSlices.splice(sourceIndex, 1);
      newSlices.push(movedSlice);
      updateOrder(newSlices);
    }
    setDraggingSliceId(null);
    setDragOverSliceId(null);
    setTimeout(() => {
      isDraggingRef.current = false;
    }, 100);
  }, [draggingSliceId, localSlices, updateOrder]);

  return {
    orderedSlices: localSlices,
    setOrderedSlices: setLocalSlices,
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
  };
};
