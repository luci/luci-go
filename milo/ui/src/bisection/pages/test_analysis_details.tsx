// Copyright 2023 The LUCI Authors.
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

import './analysis_details.css';

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useParams } from 'react-router-dom';

import { TestAnalysisOverview } from '@/bisection/components/analysis_overview';
import { CulpritVerificationTable } from '@/bisection/components/culprit_verification_table';
import { CulpritsTable } from '@/bisection/components/culprits_table/culprits_table';
import { NthSectionAnalysisTable } from '@/bisection/components/nthsection_analysis_table/nthsection_analysis_table';
import { TestFailuresTable } from '@/bisection/components/test_table';
import { useAnalysesClient } from '@/bisection/hooks/prpc_clients';
import {
  GenericCulprit,
  GenericNthSectionAnalysisResult,
  GenericSuspect,
} from '@/bisection/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { GetTestAnalysisRequest } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { TabPanel } from './analysis_details';

enum AnalysisComponentTabs {
  NTH_SECTION = 'Nth section analysis',
  CULPRIT_VERIFICATION = 'Culprit verification',
}

export function TestAnalysisDetailsPage() {
  const { id } = useParams();
  if (!id) {
    // The page should always be mounted to a path where id is set.
    throw new Error('invariant violated: id should be set');
  }
  const [currentTab, setCurrentTab] = useState(
    AnalysisComponentTabs.NTH_SECTION,
  );

  const handleTabChange = (
    _: React.SyntheticEvent,
    newTab: AnalysisComponentTabs,
  ) => {
    setCurrentTab(newTab);
  };

  const client = useAnalysesClient();
  const {
    isLoading,
    isError,
    data: analysis,
    error,
  } = useQuery(
    client.GetTestAnalysis.query(
      GetTestAnalysisRequest.fromPartial({
        analysisId: id,
      }),
    ),
  );

  if (isError) {
    return (
      <div className="section">
        <Alert severity="error">
          <AlertTitle>Failed to load analysis details</AlertTitle>
          {/* TODO: display more error detail for input issues e.g.
              Analysis not found, No analysis for that analysis ID, etc */}
          An error occurred when querying for the analysis details using
          analysis ID &quot;{id}&quot;:
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  if (isLoading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        height="80vh"
      >
        <CircularProgress />
      </Box>
    );
  }
  const suspect = analysis.nthSectionResult?.suspect
    ? [analysis.nthSectionResult.suspect]
    : [];
  return (
    <>
      <div className="section">
        <Typography variant="h5" gutterBottom>
          Analysis Details
        </Typography>
        <TestAnalysisOverview analysis={analysis} />
      </div>
      {analysis.culprit && (
        <div className="section">
          <Typography variant="h5" gutterBottom>
            Culprit Details
          </Typography>
          <CulpritsTable
            culprits={[GenericCulprit.fromTest(analysis.culprit)]}
          />
        </div>
      )}
      <div className="section">
        <Typography variant="h5" gutterBottom>
          Analysis Components
        </Typography>
        <Tabs
          value={currentTab}
          onChange={handleTabChange}
          aria-label="Analysis components tabs"
          className="rounded-tabs"
        >
          <Tab
            className="rounded-tab"
            value={AnalysisComponentTabs.NTH_SECTION}
            label={AnalysisComponentTabs.NTH_SECTION}
          />
          <Tab
            className="rounded-tab"
            value={AnalysisComponentTabs.CULPRIT_VERIFICATION}
            label={AnalysisComponentTabs.CULPRIT_VERIFICATION}
          />
        </Tabs>
        <TabPanel value={currentTab} name={AnalysisComponentTabs.NTH_SECTION}>
          <NthSectionAnalysisTable
            result={
              analysis.nthSectionResult &&
              GenericNthSectionAnalysisResult.fromTest(
                analysis.nthSectionResult,
              )
            }
          />
        </TabPanel>
        <TabPanel
          value={currentTab}
          name={AnalysisComponentTabs.CULPRIT_VERIFICATION}
        >
          <CulpritVerificationTable
            suspects={suspect.map(GenericSuspect.fromTestCulprit)}
          />
        </TabPanel>
      </div>
      {/* TODO: list the test failures. */}
      <div className="section">
        <Typography variant="h5" gutterBottom>
          Test Failures
        </Typography>
        <TestFailuresTable testFailures={analysis.testFailures} />
      </div>
    </>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="test-analysis-details">
    <TestAnalysisDetailsPage />
  </RecoverableErrorBoundary>
);
