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

import _ from 'lodash';
import { useMemo, useCallback } from 'react';

import { CHROMEOS_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import { useDeviceDimensions } from '../common/use_device_dimensions';

import { getFieldDefinition, EXTRA_COLUMN_IDS } from './chromeos_fields';

export const useChromeOSFields = () => {
  const dimensionsQuery = useDeviceDimensions({ platform: Platform.CHROMEOS });

  const availableFields = useMemo(() => {
    const ids = _.uniq([
      ...CHROMEOS_DEFAULT_COLUMNS,
      ...EXTRA_COLUMN_IDS,
      ...Object.keys(dimensionsQuery.data?.baseDimensions ?? {}),
      ...Object.keys(dimensionsQuery.data?.labels ?? {}),
    ]);

    return ids.map((id) => getFieldDefinition(id));
  }, [dimensionsQuery.data]);

  const getValues = useCallback(
    (id: string) => {
      if (!dimensionsQuery.data) return [];
      const value =
        dimensionsQuery.data.baseDimensions[id] ||
        dimensionsQuery.data.labels[id];
      return value?.values ?? [];
    },
    [dimensionsQuery.data],
  );

  return {
    availableFields,
    getValues,
    isLoading: dimensionsQuery.isPending,
  };
};
