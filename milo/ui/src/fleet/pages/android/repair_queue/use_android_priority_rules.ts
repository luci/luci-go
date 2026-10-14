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
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import {
  AndroidPriorityRule,
  CreateAndroidPriorityRuleRequest,
  DeleteAndroidPriorityRuleRequest,
  ListAndroidPriorityRulesResponse,
  UpdateAndroidPriorityRuleRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { toAndroidWorkspaceEnum } from './android_workspace';
import { ANDROID_REPAIR_QUEUE_QUERY_KEY } from './use_android_repair_queue';

export const ANDROID_PRIORITY_RULES_QUERY_KEY = ['ListAndroidPriorityRules'];

export const getAndroidPriorityRulesQueryKey = (
  workspace: AndroidPageWorkspace,
) => [...ANDROID_PRIORITY_RULES_QUERY_KEY, workspace];

const EMPTY_RULES: readonly AndroidPriorityRule[] = [];

/**
 * The rule fields the panel edits. The workspace is always the one of the
 * hook, so callers do not need to (and cannot) override it.
 */
export type AndroidPriorityRuleInput = Omit<AndroidPriorityRule, 'workspace'>;

export interface CreateAndroidPriorityRuleInput
  extends Omit<CreateAndroidPriorityRuleRequest, 'priorityRule'> {
  readonly priorityRule: AndroidPriorityRuleInput | undefined;
}

export type UpdateAndroidPriorityRuleInput = Omit<
  UpdateAndroidPriorityRuleRequest,
  'workspace'
>;

export type DeleteAndroidPriorityRuleInput = Omit<
  DeleteAndroidPriorityRuleRequest,
  'workspace'
>;

export const useCreateAndroidPriorityRule = (
  workspace: AndroidPageWorkspace,
) => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const protoWorkspace = toAndroidWorkspaceEnum(workspace);

  return useMutation({
    // `priority_rule.workspace` is the only place the backend reads the
    // workspace from on create.
    mutationFn: (req: CreateAndroidPriorityRuleInput) =>
      client.CreateAndroidPriorityRule({
        ...req,
        priorityRule: req.priorityRule
          ? { ...req.priorityRule, workspace: protoWorkspace }
          : undefined,
      }),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: ANDROID_PRIORITY_RULES_QUERY_KEY,
      });
      queryClient.invalidateQueries({
        queryKey: ANDROID_REPAIR_QUEUE_QUERY_KEY,
      });
    },
  });
};

export const useUpdateAndroidPriorityRule = (
  workspace: AndroidPageWorkspace,
) => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const protoWorkspace = toAndroidWorkspaceEnum(workspace);
  const queryKey = getAndroidPriorityRulesQueryKey(workspace);

  return useMutation({
    mutationFn: (req: UpdateAndroidPriorityRuleInput) =>
      client.UpdateAndroidPriorityRule({ ...req, workspace: protoWorkspace }),
    onMutate: async (updateReq) => {
      await queryClient.cancelQueries({
        queryKey,
      });
      const previousData =
        queryClient.getQueryData<ListAndroidPriorityRulesResponse>(queryKey);
      if (previousData) {
        queryClient.setQueryData<ListAndroidPriorityRulesResponse>(queryKey, {
          ...previousData,
          priorityRules: previousData.priorityRules.map((rule) =>
            rule.id === updateReq.id
              ? {
                  ...rule,
                  expressionAip160:
                    updateReq.expressionAip160 ?? rule.expressionAip160,
                  weight: updateReq.weight ?? rule.weight,
                }
              : rule,
          ),
        });
      }
      return { previousData };
    },
    onError: (_err, _variables, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(queryKey, context.previousData);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: ANDROID_PRIORITY_RULES_QUERY_KEY,
      });
      queryClient.invalidateQueries({
        queryKey: ANDROID_REPAIR_QUEUE_QUERY_KEY,
      });
    },
  });
};

export const useDeleteAndroidPriorityRule = (
  workspace: AndroidPageWorkspace,
) => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const protoWorkspace = toAndroidWorkspaceEnum(workspace);
  const queryKey = getAndroidPriorityRulesQueryKey(workspace);

  return useMutation({
    mutationFn: (req: DeleteAndroidPriorityRuleInput) =>
      client.DeleteAndroidPriorityRule({ ...req, workspace: protoWorkspace }),
    onMutate: async (deleteReq) => {
      await queryClient.cancelQueries({
        queryKey,
      });
      const previousData =
        queryClient.getQueryData<ListAndroidPriorityRulesResponse>(queryKey);
      if (previousData) {
        queryClient.setQueryData<ListAndroidPriorityRulesResponse>(queryKey, {
          ...previousData,
          priorityRules: previousData.priorityRules.filter(
            (rule) => rule.id !== deleteReq.id,
          ),
        });
      }
      return { previousData };
    },
    onError: (_err, _variables, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(queryKey, context.previousData);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: ANDROID_PRIORITY_RULES_QUERY_KEY,
      });
      queryClient.invalidateQueries({
        queryKey: ANDROID_REPAIR_QUEUE_QUERY_KEY,
      });
    },
  });
};

export const useAndroidPriorityRules = (workspace: AndroidPageWorkspace) => {
  const client = useFleetConsoleClient();
  const protoWorkspace = toAndroidWorkspaceEnum(workspace);
  const queryKey = getAndroidPriorityRulesQueryKey(workspace);

  const rulesQuery = useQuery({
    ...client.ListAndroidPriorityRules.query({
      workspace: protoWorkspace,
    }),
    queryKey,
  });

  const createRuleMutation = useCreateAndroidPriorityRule(workspace);
  const updateRuleMutation = useUpdateAndroidPriorityRule(workspace);
  const deleteRuleMutation = useDeleteAndroidPriorityRule(workspace);

  return {
    rules: rulesQuery.data?.priorityRules ?? EMPTY_RULES,
    isLoading: rulesQuery.isLoading,
    isError: rulesQuery.isError,
    error: rulesQuery.error,
    refetch: rulesQuery.refetch,
    createRule: createRuleMutation.mutateAsync,
    isCreating: createRuleMutation.isPending,
    createError: createRuleMutation.error,
    updateRule: updateRuleMutation.mutateAsync,
    isUpdating: updateRuleMutation.isPending,
    updateError: updateRuleMutation.error,
    deleteRule: deleteRuleMutation.mutateAsync,
    isDeleting: deleteRuleMutation.isPending,
    deleteError: deleteRuleMutation.error,
  };
};
