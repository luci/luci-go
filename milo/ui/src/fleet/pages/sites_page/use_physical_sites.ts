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
import { useMemo } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  CreatePhysicalSiteRequest,
  DeletePhysicalSiteRequest,
  PhysicalSite,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const PHYSICAL_SITES_QUERY_KEY = ['ListPhysicalSites'] as const;

export const usePhysicalSites = () => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();

  const sitesQuery = useQuery({
    ...client.ListPhysicalSites.query({}),
    queryKey: PHYSICAL_SITES_QUERY_KEY,
    retry: false,
  });

  const sites = useMemo(
    () => sitesQuery.data?.physicalSites ?? [],
    [sitesQuery.data],
  );

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: PHYSICAL_SITES_QUERY_KEY });

  const createSiteMutation = useMutation({
    mutationFn: (physicalSite: PhysicalSite) =>
      client.CreatePhysicalSite(
        CreatePhysicalSiteRequest.fromPartial({ physicalSite }),
      ),
    onSuccess: invalidate,
  });

  const deleteSiteMutation = useMutation({
    mutationFn: (id: string) =>
      client.DeletePhysicalSite(DeletePhysicalSiteRequest.fromPartial({ id })),
    onSuccess: invalidate,
  });

  return {
    sites,
    isLoading: sitesQuery.isLoading,
    isError: sitesQuery.isError,
    error: sitesQuery.error,
    createSiteMutation,
    deleteSiteMutation,
  };
};
