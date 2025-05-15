// Copyright 2022 The LUCI Authors.
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

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useContext } from 'react';

import { SnackbarContext } from '@/clusters/context/snackbar_context';
import { useRulesService } from '@/clusters/services/services';
import { UpdateRuleRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

type MutationCallback = () => void;

export const useMutateRule = (
  successCallback?: MutationCallback,
  errorCallback?: MutationCallback,
) => {
  const ruleService = useRulesService();
  const queryClient = useQueryClient();
  const { setSnack } = useContext(SnackbarContext);

  return useMutation({
    mutationFn: (updateRuleRequest: UpdateRuleRequest) =>
      ruleService.Update(updateRuleRequest),

    onSuccess: (data) => {
      queryClient.setQueryData(['rules', data.project, data.ruleId], data);
      setSnack({
        open: true,
        message: 'Rule updated successfully',
        severity: 'success',
      });
      if (successCallback) {
        successCallback();
      }
    },

    onError: (error) => {
      setSnack({
        open: true,
        message: `Failed to mutate rule due to: ${error}`,
        severity: 'error',
      });
      if (errorCallback) {
        errorCallback();
      }
    },
  });
};
