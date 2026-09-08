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

import { useAdminTaskPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

export const DEFAULT_QUOTA_QUERY_KEY = ['GetDefaultQuota'];

export const useDefaultQuota = () => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const { hasPermission } = useAdminTaskPermission();

  const quotaQuery = useQuery({
    ...client.GetDefaultQuota.query({}),
    queryKey: DEFAULT_QUOTA_QUERY_KEY,
  });

  const setQuotaMutation = useMutation({
    mutationFn: (defaultQuota: number) =>
      client.SetDefaultQuota({ defaultQuota }),
    onSuccess: (data) => {
      queryClient.setQueryData(DEFAULT_QUOTA_QUERY_KEY, data);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: DEFAULT_QUOTA_QUERY_KEY });
    },
  });

  return {
    quotaQuery,
    setQuotaMutation,
    canEdit: hasPermission ?? false,
    isPermissionLoading: hasPermission === null,
  };
};
