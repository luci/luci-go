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

import { useEffect } from 'react';

import { OptionComponentProps } from '@/fleet/components/filter_dropdown/filter_dropdown_old';
import { OptionsMenu } from '@/fleet/components/filter_dropdown/options_menu';

import { getFulfillmentStatusScoredOptions } from './fulfillment_status';
import {
  ResourceRequestInsightsOptionComponentProps,
  RriFilters,
} from './use_rri_filters';

export const FulfillmentStatusFilter = ({
  optionComponentProps: { onFiltersChange, onClose, filters },
  searchQuery,
}: OptionComponentProps<ResourceRequestInsightsOptionComponentProps>) => {
  useEffect(() => () => onClose(), [onClose]);

  const getFiltersAfterFlip = (value: string): RriFilters => {
    if (filters?.fulfillment_status?.includes(value)) {
      if (filters.fulfillment_status.length === 1) {
        return {
          ...filters,
          fulfillment_status: undefined,
        };
      }

      return {
        ...filters,
        fulfillment_status: filters.fulfillment_status.filter(
          (x) => x !== value,
        ),
      };
    }

    return {
      ...filters,
      fulfillment_status: [...(filters?.fulfillment_status ?? []), value],
    };
  };

  return (
    <div css={{ marginTop: 8 }}>
      <OptionsMenu
        elements={getFulfillmentStatusScoredOptions(searchQuery)}
        selectedElements={new Set(filters?.fulfillment_status ?? [])}
        flipOption={(value: string) => {
          onFiltersChange(getFiltersAfterFlip(value));
        }}
      />
    </div>
  );
};
