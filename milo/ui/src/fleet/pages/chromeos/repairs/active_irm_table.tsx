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

import { ActiveIrmTable as SharedActiveIrmTable } from '@/fleet/components/repair_queue/active_irm_table';

import { useIrmIncidents } from './use_irm_incidents';

export const ActiveIrmTable = () => {
  const { data, isLoading, isError, error } = useIrmIncidents();

  const incidents = data?.irmIncidents ?? [];

  return (
    <SharedActiveIrmTable
      incidents={incidents}
      isLoading={isLoading}
      isError={isError}
      error={error}
    />
  );
};
