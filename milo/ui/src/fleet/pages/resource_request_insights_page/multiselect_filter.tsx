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

import styled from '@emotion/styled';
import { Alert, CircularProgress, MenuList } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';

import { OptionComponentProps } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { OptionsMenu } from '@/fleet/components/filter_dropdown/options_menu';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

import {
  getSortedMultiselectElements,
  ResourceRequestInsightsOptionComponentProps,
  RriFilters,
} from './use_rri_filters';

const Container = styled.div`
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100%;
  width: 100%;
  padding: 10px;
  box-sizing: border-box;
`;

export const MultiSelectFilter = ({
  optionComponentProps: { onFiltersChange, onClose, filters, option },
  searchQuery,
}: OptionComponentProps<ResourceRequestInsightsOptionComponentProps>) => {
  useEffect(() => () => onClose(), [onClose]);

  const [initialSelections, _] = useState(
    (filters && (filters[option.value] as string[])) ?? [],
  );

  const client = useFleetConsoleClient();
  const query = useQuery(
    client.GetResourceRequestsMultiselectFilterValues.query({}),
  );

  if (query.isLoading) {
    return (
      <Container>
        <CircularProgress />
      </Container>
    );
  }

  if (query.isError) {
    return (
      <Container>
        <Alert css={{ width: '100%' }} severity="error">
          {query.error.message}
        </Alert>
      </Container>
    );
  }

  if (!query.data) {
    return (
      <Container>
        <Alert css={{ width: '100%' }} severity="error">
          Unexpected error
        </Alert>
      </Container>
    );
  }

  const elements = getSortedMultiselectElements(
    query.data,
    option.value,
    searchQuery,
    initialSelections,
  );

  const optionValues = filters && (filters[option.value] as string[]);

  const getFiltersAfterFlip = (value: string): RriFilters => {
    if (optionValues?.includes(value)) {
      if (optionValues.length === 1) {
        return {
          ...filters,
          [option.value]: undefined,
        };
      }

      return {
        ...filters,
        [option.value]: optionValues.filter((x) => x !== value),
      };
    }

    return {
      ...filters,
      [option.value]: [...(optionValues ?? []), value],
    };
  };

  return (
    <MenuList
      variant="selectedMenu"
      sx={{
        maxHeight: 400,
        width: 300,
      }}
    >
      <OptionsMenu
        elements={elements}
        selectedElements={new Set(optionValues ?? [])}
        flipOption={(value: string) => {
          onFiltersChange(getFiltersAfterFlip(value));
        }}
      />
    </MenuList>
  );
};
