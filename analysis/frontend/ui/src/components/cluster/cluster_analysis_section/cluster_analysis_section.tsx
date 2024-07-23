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
  useContext,
} from 'react';
import {
  useLocation,
  useSearchParams,
} from 'react-router-dom';

import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Paper from '@mui/material/Paper';
import Tab from '@mui/material/Tab';
import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';

import FailuresTab from '@/components/cluster/cluster_analysis_section/failures_tab/failures_tab';
import OverviewTab from '@/components/cluster/cluster_analysis_section/overview_tab/overview_tab';
import ExonerationsTab from '@/components/cluster/cluster_analysis_section/exonerations_tab/exonerations_tab';

import { ClusterContext } from '../cluster_context';
import ExonerationsV2Tab from './exonerations_v2_tab/exonerations_v2_tab';


const ClusterAnalysisSection = () => {
  const {
    project,
  } = useContext(ClusterContext);
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();

  const handleTabChange = (newValue: string) => {
    setSearchParams((params) => {
      params.set('tab', newValue);
      return params;
    }, { replace: true });
  };

  const exonerationV1Projects = ['chromeos'];
  const exonerationV2Projects = ['chromium'];
  const exonerationV1Available = exonerationV1Projects.includes(project);
  const exonerationV2Available = exonerationV2Projects.includes(project);

  const validValues = ['overview', 'recent-failures'];
  if (exonerationV1Available || exonerationV2Available) {
    validValues.push('exonerations');
  }

  // Handle legacy URLs that used hash instead of search params.
  let hashValue = location.hash;
  if (hashValue.length > 0) {
    // Cut off the leading '#'.
    hashValue = hashValue.slice(1);
  }
  if (validValues.indexOf(hashValue) != -1) {
    handleTabChange(hashValue);
  }

  let value = searchParams.get('tab');
  if (!value || validValues.indexOf(value) == -1) {
    value = 'overview';
  }

  return (
    <Paper
      data-cy="analysis-section"
      elevation={3}
      sx={{ pt: 2, pb: 2, mt: 1 }}
    >
      <Container maxWidth={false}>
        <TabContext value={value}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <TabList value={value} onChange={(_, newValue) => handleTabChange(newValue)}>
              <Tab label='Overview' value='overview' />
              <Tab label='Recent Failures' value='recent-failures' />
              {
                (exonerationV1Available || exonerationV2Available) && (
                  <Tab label='Exonerations' value='exonerations' />
                )
              }
            </TabList>
          </Box>
          <OverviewTab value='overview'/>
          <FailuresTab value='recent-failures'/>
          {
            exonerationV1Available && (
              <ExonerationsTab value='exonerations'/>
            )
          }
          {
            exonerationV2Available && (
              <ExonerationsV2Tab value='exonerations'/>
            )
          }
        </TabContext>
      </Container>
    </Paper>
  );
};

export default ClusterAnalysisSection;
