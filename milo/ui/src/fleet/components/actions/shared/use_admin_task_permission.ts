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

import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';

import { logging } from '@/common/tools/logging';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

export function usePermission(group: string) {
  const fleetConsoleClient = useFleetConsoleClient();

  const queryOptions = fleetConsoleClient.CheckPermission.query({
    group: group,
  });

  const { data, isError, error } = useQuery({
    ...queryOptions,
    staleTime: 1 * 60 * 1000, // 1 minute
    retry: false,
  });

  useEffect(() => {
    if (isError && error) {
      logging.error(`Failed to check permission for ${group}:`, error);
    }
  }, [isError, error, group]);

  if (isError) {
    return false;
  }

  return data?.hasPermission ?? null;
}

export function useAdminTaskPermission() {
  return usePermission('mdb/fleet-console-admin-tasks-policy');
}
