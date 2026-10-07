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

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const HEALTH_SLICES_QUERY_KEY = ['ListHealthSlices'];

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

  return {
    slicesQuery,
    createSliceMutation,
    updateSliceMutation,
    deleteSliceMutation,
  };
};
