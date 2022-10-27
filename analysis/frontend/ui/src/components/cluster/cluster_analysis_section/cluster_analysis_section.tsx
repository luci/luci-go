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

import {
  useLocation,
  useNavigate,
} from 'react-router-dom';

import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Paper from '@mui/material/Paper';
import Tab from '@mui/material/Tab';
import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';

import FailuresTab from '@/components/cluster/cluster_analysis_section/failures_tab/failures_tab';
import OverviewTab from '@/components/cluster/cluster_analysis_section/overview_tab/overview_tab';

const ClusterAnalysisSection = () => {
  const location = useLocation();
  const navigate = useNavigate();

  const handleTabChange = (_: React.SyntheticEvent, newValue: string) => {
    navigate({ hash: '#' + newValue }, { replace: true });
  };

  let value = location.hash;
  if (value.length > 0) {
    // Cut off the leading '#'.
    value = value.slice(1);
  }
  if (value != 'overview' && value != 'recent-failures') {
    // Default to a tab we know.
    value = 'overview';
  }

  return (
    <Paper data-cy="analysis-section" elevation={3} sx={{ pt: 2, pb: 2, mt: 1 }} >
      <Container maxWidth={false}>
        <TabContext value={value}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <TabList value={value} onChange={handleTabChange}>
              <Tab label='Overview' value='overview' />
              <Tab label='Recent Failures' value='recent-failures' />
            </TabList>
          </Box>
          <OverviewTab value='overview'/>
          <FailuresTab value='recent-failures'/>
        </TabContext>
      </Container>
    </Paper>
  );
};

export default ClusterAnalysisSection;
