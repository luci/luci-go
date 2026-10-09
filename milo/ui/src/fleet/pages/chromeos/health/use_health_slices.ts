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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';

import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const HEALTH_SLICES_QUERY_KEY = ['ListHealthSlices'];
export const DEFAULT_SLICE_STORAGE_KEY =
  'fleet.chromeos.health.default_slice_id';

export const useDefaultSlice = () => {
  const [defaultSliceId, setDefaultSliceIdState] = useState<string | null>(
    () => {
      try {
        return localStorage.getItem(DEFAULT_SLICE_STORAGE_KEY);
      } catch {
        return null;
      }
    },
  );

  const setDefaultSliceId = useCallback((id: string | null) => {
    setDefaultSliceIdState(id);
    try {
      if (id === null) {
        localStorage.removeItem(DEFAULT_SLICE_STORAGE_KEY);
      } else {
        localStorage.setItem(DEFAULT_SLICE_STORAGE_KEY, id);
      }
    } catch {
      // Ignore localStorage errors (e.g. private mode, sandbox restrictions).
    }
  }, []);

  return { defaultSliceId, setDefaultSliceId };
};

/**
 * Automatically applies the pinned default slice filter to the URL if the user
 * lands on the page with no active ?filters= query param.
 */
export const useDefaultSliceSync = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const { defaultSliceId, setDefaultSliceId } = useDefaultSlice();
  const { slicesQuery } = useHealthSlices();
  const hasAppliedDefaultRef = useRef(false);

  useEffect(() => {
    if (hasAppliedDefaultRef.current) return;
    if (!defaultSliceId) {
      hasAppliedDefaultRef.current = true;
      return;
    }
    if (!slicesQuery.data) return;

    hasAppliedDefaultRef.current = true;
    const defaultSlice = slicesQuery.data.healthSlices?.find(
      (s) => s.id === defaultSliceId,
    );
    if (!defaultSlice) {
      // Auto-purge stale defaultSliceId if the slice was deleted.
      setDefaultSliceId(null);
      return;
    }

    if (!searchParams.has(FILTERS_PARAM_KEY)) {
      if (defaultSlice.filter.trim()) {
        setSearchParams(
          (prev) => {
            const next = new URLSearchParams(prev);
            next.set(FILTERS_PARAM_KEY, defaultSlice.filter.trim());
            return next;
          },
          { replace: true },
        );
      }
    }
  }, [
    defaultSliceId,
    setDefaultSliceId,
    slicesQuery.data,
    searchParams,
    setSearchParams,
  ]);
};

export const useHealthSlices = () => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();

  const slicesQuery = useQuery({
    ...client.ListHealthSlices.query({}),
    queryKey: HEALTH_SLICES_QUERY_KEY,
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: HEALTH_SLICES_QUERY_KEY });

  const createSliceMutation = useMutation({
    mutationFn: (healthSlice: HealthSlice) =>
      client.CreateHealthSlice({ healthSlice }),
    onSettled: invalidate,
  });

  const updateSliceMutation = useMutation({
    mutationFn: (healthSlice: HealthSlice) =>
      client.UpdateHealthSlice({ healthSlice }),
    onSettled: invalidate,
  });

  const deleteSliceMutation = useMutation({
    mutationFn: (id: string) => client.DeleteHealthSlice({ id }),
    onSettled: invalidate,
  });

  const reorderSlicesMutation = useMutation({
    mutationFn: (sliceIds: readonly string[]) =>
      client.ReorderHealthSlices({ sliceIds }),
    onSuccess: (data) => {
      queryClient.setQueryData(HEALTH_SLICES_QUERY_KEY, data);
    },
    onSettled: invalidate,
  });

  return {
    slicesQuery,
    createSliceMutation,
    updateSliceMutation,
    deleteSliceMutation,
    reorderSlicesMutation,
  };
};
