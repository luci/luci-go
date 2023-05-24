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

import TabPanel from '@mui/lab/TabPanel';
import Divider from '@mui/material/Divider';
import Stack from '@mui/material/Stack';

import { HistoryCharts } from './history_charts/history_charts';
import { RecommendedPrioritySection } from './recommended_priority/recommended_priority_section';

interface Props {
  // The name of the tab.
  value: string;
}

const OverviewTab = ({ value }: Props) => {
  return (
    <TabPanel value={value}>
      <Stack direction="column" spacing={4}>
        <RecommendedPrioritySection />
        <Divider />
        <HistoryCharts />
      </Stack>
    </TabPanel>
  );
};

export default OverviewTab;
