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
import {
  CreatePriorityRuleRequest,
  DeletePriorityRuleRequest,
  ListPriorityRulesResponse,
  PriorityRule,
  UpdatePriorityRuleRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { REPAIR_QUEUE_QUERY_KEY } from './use_repair_queue';

export const PRIORITY_RULES_QUERY_KEY = ['ListPriorityRules'];

const EMPTY_RULES: readonly PriorityRule[] = [];

export const usePriorityRules = () => {
  const client = useFleetConsoleClient();
  const queryClient = useQueryClient();

  const rulesQuery = useQuery({
    ...client.ListPriorityRules.query({}),
    queryKey: PRIORITY_RULES_QUERY_KEY,
  });

  const invalidateRepairQueueQueries = () => {
    queryClient.invalidateQueries({
      queryKey: REPAIR_QUEUE_QUERY_KEY,
    });
  };

  const createRuleMutation = useMutation({
    mutationFn: (req: CreatePriorityRuleRequest) =>
      client.CreatePriorityRule(req),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: PRIORITY_RULES_QUERY_KEY });
      invalidateRepairQueueQueries();
    },
  });

  const updateRuleMutation = useMutation({
    mutationFn: (req: UpdatePriorityRuleRequest) =>
      client.UpdatePriorityRule(req),
    onMutate: async (updatedReq) => {
      await queryClient.cancelQueries({ queryKey: PRIORITY_RULES_QUERY_KEY });
      const previousData = queryClient.getQueryData<ListPriorityRulesResponse>(
        PRIORITY_RULES_QUERY_KEY,
      );
      if (previousData) {
        queryClient.setQueryData<ListPriorityRulesResponse>(
          PRIORITY_RULES_QUERY_KEY,
          {
            ...previousData,
            priorityRules: previousData.priorityRules.map((rule) => {
              if (rule.id === updatedReq.id) {
                return {
                  ...rule,
                  expressionAip160:
                    updatedReq.expressionAip160 !== undefined
                      ? updatedReq.expressionAip160
                      : rule.expressionAip160,
                  weight:
                    updatedReq.weight !== undefined
                      ? updatedReq.weight
                      : rule.weight,
                };
              }
              return rule;
            }),
          },
        );
      }
      return { previousData };
    },
    onError: (_err, _variables, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          PRIORITY_RULES_QUERY_KEY,
          context.previousData,
        );
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: PRIORITY_RULES_QUERY_KEY });
      invalidateRepairQueueQueries();
    },
  });

  const deleteRuleMutation = useMutation({
    mutationFn: (req: DeletePriorityRuleRequest) =>
      client.DeletePriorityRule(req),
    onMutate: async (deleteReq) => {
      await queryClient.cancelQueries({ queryKey: PRIORITY_RULES_QUERY_KEY });
      const previousData = queryClient.getQueryData<ListPriorityRulesResponse>(
        PRIORITY_RULES_QUERY_KEY,
      );
      if (previousData) {
        queryClient.setQueryData<ListPriorityRulesResponse>(
          PRIORITY_RULES_QUERY_KEY,
          {
            ...previousData,
            priorityRules: previousData.priorityRules.filter(
              (rule) => rule.id !== deleteReq.id,
            ),
          },
        );
      }
      return { previousData };
    },
    onError: (_err, _variables, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          PRIORITY_RULES_QUERY_KEY,
          context.previousData,
        );
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: PRIORITY_RULES_QUERY_KEY });
      invalidateRepairQueueQueries();
    },
  });

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
