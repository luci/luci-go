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
import { useContext } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { ClusterContext } from '@/clusters/components/cluster/cluster_context';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchMetrics from '@/clusters/hooks/use_fetch_metrics';

import { HistoryChartsSection } from './history_charts/history_charts_section';
import { OverviewTabContextProvider } from './overview_tab_context';
import { ProblemsSection } from './problems_section/problems_section';

interface Props {
  // The name of the tab.
  value: string;
}

const OverviewTab = ({ value }: Props) => {
  const { project, algorithm } = useContext(ClusterContext);

  const {
    isLoading,
    isSuccess,
    data: metrics,
    error,
  } = useFetchMetrics(project);

  return (
    <OverviewTabContextProvider metrics={metrics}>
      <TabPanel value={value}>
        {error && <LoadErrorAlert entityName="metrics" error={error} />}
        {isLoading && <CentralizedProgress />}
        {isSuccess && metrics !== undefined && (
          <Stack direction="column" spacing={4}>
            {algorithm === 'rules' && (
              <>
                <ProblemsSection />
                <Divider />
              </>
            )}
            <HistoryChartsSection />
          </Stack>
        )}
      </TabPanel>
    </OverviewTabContextProvider>
  );
};

export default OverviewTab;
