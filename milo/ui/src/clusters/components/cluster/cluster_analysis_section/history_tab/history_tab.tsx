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
import { useContext } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { HistoryTabContextProvider } from '@/clusters/components/cluster/cluster_analysis_section/history_tab/provider';
import { ClusterContext } from '@/clusters/components/cluster/context';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchMetrics from '@/clusters/hooks/use_fetch_metrics';

import { HistoryChartsSection } from './history_charts/history_charts_section';

interface Props {
  // The name of the tab.
  value: string;
}

const HistoryTab = ({ value }: Props) => {
  const { project } = useContext(ClusterContext);

  const {
    isLoading,
    isSuccess,
    data: metrics,
    error,
  } = useFetchMetrics(project);

  return (
    <HistoryTabContextProvider metrics={metrics}>
      <TabPanel value={value}>
        {error && <LoadErrorAlert entityName="metrics" error={error} />}
        {isLoading && <CentralizedProgress />}
        {isSuccess && metrics !== undefined && <HistoryChartsSection />}
      </TabPanel>
    </HistoryTabContextProvider>
  );
};

export default HistoryTab;
