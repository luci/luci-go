// Copyright 2025 The LUCI Authors.
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
  useMutation,
  UseMutationResult,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

import { useAlertGroupsClient } from '@/monitoringv2/hooks/prpc_clients';
import {
  AlertGroup,
  ListAlertGroupsResponse,
  UpdateAlertGroupRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';
import { Empty } from '@/proto/google/protobuf/empty.pb';

import { useTree } from '../pages/monitoring_page/context';

export interface AlertGroupCreateData {
  displayName: string;
  statusMessage: string;
  alertKeys: string[];
  bugs?: string[];
}

export interface AlertGroupsData {
  alertGroups: readonly AlertGroup[];
  create: UseMutationResult<AlertGroup, Error, AlertGroupCreateData, unknown>;
  update: UseMutationResult<
    AlertGroup,
    Error,
    UpdateAlertGroupRequest,
    unknown
  >;
  delete: UseMutationResult<Empty, Error, AlertGroup, unknown>;
}

export const useAlertGroups = (): AlertGroupsData => {
  const tree = useTree();
  const queryClient = useQueryClient();
  const alertGroupsClient = useAlertGroupsClient();
  const listQuery = alertGroupsClient.ListAlertGroups.query({
    parent: `rotations/${tree?.name}`,
    pageSize: 1000,
    pageToken: '',
  });
  const { data: alertGroupsResponse } = useQuery(listQuery);
  const updateAndInvalidateList = (
    updateFn: (old: ListAlertGroupsResponse) => ListAlertGroupsResponse,
  ) => {
    queryClient.setQueryData(listQuery.queryKey, updateFn);
    queryClient.invalidateQueries({ queryKey: listQuery.queryKey });
  };

  const createGroup = useMutation({
    mutationFn: (group: AlertGroupCreateData) =>
      alertGroupsClient.CreateAlertGroup({
        parent: `rotations/${tree?.name}`,
        alertGroupId: new Date().getTime().toString(),
        alertGroup: { ...(group as Partial<AlertGroup>) } as AlertGroup, // Some fields are meant to be left empty on create.
      }),
    onSuccess: (group) => {
      updateAndInvalidateList((old) => ({
        ...old,
        alertGroups: [...(old?.alertGroups || []), group],
      }));
    },
  });

  const updateGroup = useMutation({
    mutationFn: (request: UpdateAlertGroupRequest) =>
      alertGroupsClient.UpdateAlertGroup(request),
    onSuccess: (group) => {
      updateAndInvalidateList((old) => ({
        ...old,
        alertGroups: (old?.alertGroups || []).map((g) =>
          g.name === group.name ? group : g,
        ),
      }));
    },
  });

  const deleteGroup = useMutation({
    mutationFn: (group: AlertGroup) =>
      alertGroupsClient.DeleteAlertGroup({
        name: group.name,
      }),
    onSuccess: (_ignored, group) => {
      updateAndInvalidateList((old) => ({
        ...old,
        alertGroups: [
          ...(old?.alertGroups || []).filter((g) => g.name !== group.name),
        ],
      }));
    },
  });

  return {
    alertGroups: alertGroupsResponse?.alertGroups || [],
    create: createGroup,
    update: updateGroup,
    delete: deleteGroup,
  };
};
