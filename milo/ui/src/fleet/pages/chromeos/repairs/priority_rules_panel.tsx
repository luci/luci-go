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

import { usePriorityRulesPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { PriorityRulesPanel as SharedPriorityRulesPanel } from '@/fleet/components/repair_queue/priority_rules_panel';
import { useChromeOSFilterBuilders } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_filters';

import { usePriorityRules } from './use_priority_rules';

export const PriorityRulesPanel = () => {
  const {
    rules,
    isLoading,
    isError,
    error,
    createRule,
    isCreating,
    updateRule,
    isUpdating,
    deleteRule,
    isDeleting,
  } = usePriorityRules();

  const { filterBuilders, isLoading: isBuildersLoading } =
    useChromeOSFilterBuilders();
  const { hasPermission } = usePriorityRulesPermission();
  const canEdit = hasPermission === true;

  return (
    <SharedPriorityRulesPanel
      rules={rules}
      isLoading={isLoading}
      isError={isError}
      error={error}
      createRule={createRule}
      isCreating={isCreating}
      updateRule={updateRule}
      isUpdating={isUpdating}
      deleteRule={deleteRule}
      isDeleting={isDeleting}
      filterBuilders={filterBuilders}
      isBuildersLoading={isBuildersLoading}
      canEdit={canEdit}
      searchPlaceholder="Add rule filter (e.g. pool, board, model)..."
    />
  );
};
