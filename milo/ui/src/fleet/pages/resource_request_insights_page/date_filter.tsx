// Copyright 2025 The LUCI Authors.
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

import { useEffect } from 'react';

import { OptionComponentProps } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { toIsoString } from '@/fleet/utils/dates';

import { ResourceRequestInsightsOptionComponentProps } from './resource_request_insights_page';

export const DateFilter = ({
  optionComponentProps: { onFiltersChange, onClose, filters },
}: OptionComponentProps<ResourceRequestInsightsOptionComponentProps>) => {
  useEffect(() => () => onClose(), [onClose]);

  return (
    <>
      <h1>Material Sourcing Target Delivery Date</h1>
      <button
        onClick={() =>
          onFiltersChange({
            material_sourcing_target_delivery_date: {
              min: {
                year: 2025,
                month: 4,
                day: 1,
              },
            },
          })
        }
      >
        Filter
      </button>
      {filters.material_sourcing_target_delivery_date?.min && (
        <span>
          Later than:{' '}
          {toIsoString(filters.material_sourcing_target_delivery_date.min)}
        </span>
      )}
    </>
  );
};
