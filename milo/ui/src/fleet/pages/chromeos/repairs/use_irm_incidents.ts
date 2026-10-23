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
import { useCallback, useState } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ListIrmIncidentsRequest,
  ListIrmIncidentsResponse,
  UpdateIrmIncidentRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { REPAIR_QUEUE_QUERY_KEY } from './use_repair_queue';

export const IRM_INCIDENTS_QUERY_KEY = ['ListIrmIncidents'] as const;

const DEFAULT_LIST_REQUEST: ListIrmIncidentsRequest = {};

export const useIrmIncidents = (
  request: ListIrmIncidentsRequest = DEFAULT_LIST_REQUEST,
) => {
  const client = useFleetConsoleClient();
  return useQuery({
    ...client.ListIrmIncidents.query(request),
    queryKey: [...IRM_INCIDENTS_QUERY_KEY, request],
    refetchInterval: 10000,
  });
};

export const useUpdateIrmIncident = () => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const [syncingIds, setSyncingIds] = useState<ReadonlySet<string>>(new Set());

  const mutation = useMutation({
    mutationFn: (req: UpdateIrmIncidentRequest) =>
      client.UpdateIrmIncident(req),
    onMutate: async (updatedReq) => {
      setSyncingIds((prev) => {
        const next = new Set(prev);
        next.add(updatedReq.id);
        return next;
      });

      await queryClient.cancelQueries({ queryKey: IRM_INCIDENTS_QUERY_KEY });
      const previousQueries =
        queryClient.getQueriesData<ListIrmIncidentsResponse>({
          queryKey: IRM_INCIDENTS_QUERY_KEY,
        });

      const normalizedExprs = (updatedReq.expressionsAip160 ?? []).map((e) =>
        e.trim(),
      );
      const isUnlinked = normalizedExprs.length === 0;
      queryClient.setQueriesData<ListIrmIncidentsResponse>(
        { queryKey: IRM_INCIDENTS_QUERY_KEY },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            irmIncidents: old.irmIncidents.map((incident) => {
              if (incident.id !== updatedReq.id) {
                return incident;
              }
              return {
                ...incident,
                expressionsAip160: normalizedExprs,
                affectedDevicesCount: isUnlinked
                  ? 0
                  : incident.affectedDevicesCount,
              };
            }),
          };
        },
      );

      return { previousQueries };
    },
    onError: (_err, _variables, context) => {
      if (context?.previousQueries) {
        for (const [key, data] of context.previousQueries) {
          queryClient.setQueryData(key, data);
        }
      }
    },
    onSuccess: (response, updatedReq) => {
      if (response.irmIncident) {
        queryClient.setQueriesData<ListIrmIncidentsResponse>(
          { queryKey: IRM_INCIDENTS_QUERY_KEY },
          (old) => {
            if (!old) return old;
            return {
              ...old,
              irmIncidents: old.irmIncidents.map((incident) =>
                incident.id === updatedReq.id && response.irmIncident
                  ? response.irmIncident
                  : incident,
              ),
            };
          },
        );
      }
    },
    onSettled: async (_data, _error, updatedReq) => {
      try {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: IRM_INCIDENTS_QUERY_KEY }),
          queryClient.invalidateQueries({ queryKey: REPAIR_QUEUE_QUERY_KEY }),
        ]);
      } finally {
        setSyncingIds((prev) => {
          const next = new Set(prev);
          if (updatedReq?.id) {
            next.delete(updatedReq.id);
          }
          return next;
        });
      }
    },
  });

  const isSyncingIncident = useCallback(
    (id: string) => syncingIds.has(id),
    [syncingIds],
  );

  return {
    updateIncident: mutation.mutateAsync,
    isUpdating: mutation.isPending,
    updateError: mutation.error,
    syncingIncidentIds: syncingIds,
    isSyncingIncident,
  };
};
