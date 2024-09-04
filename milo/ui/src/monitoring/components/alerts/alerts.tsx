// Copyright 2024 The LUCI Authors.
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

import { Box } from '@mui/material';

import { SearchInput } from '@/common/components/search_input';
import { useTree } from '@/monitoring/pages/monitoring_page/context';

import { AlertTabs } from './alert_tabs';
import { useFilterQuery } from './hooks';
import { TreeStatusCard } from './tree_status_card';

function SearchAlertInput() {
  const [filter, updateFilter] = useFilterQuery('');
  return (
    <SearchInput
      initDelayMs={200}
      onValueChange={(value) => updateFilter(value)}
      size="small"
      value={filter}
      placeholder="Filter Alerts - hint: use this to perform operations like linking all matching alerts to a bug"
    />
  );
}

export const Alerts = () => {
  const tree = useTree();

  if (!tree) {
    return <></>;
  }

  return (
    <>
      <Box
        sx={{
          position: 'sticky',
          top: 48,
          zIndex: 30,
          backgroundColor: '#fff',
          padding: '8px 16px',
          boxShadow: 3,
        }}
      >
        <SearchAlertInput />
      </Box>
      <div css={{ margin: '16px 0', padding: '0 16px' }}>
        <TreeStatusCard tree={tree} />
        <AlertTabs />
      </div>
    </>
  );
};
