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

import { AnalysisOverview } from '@/bisection/components/analysis_overview';
import { CulpritVerificationTable } from '@/bisection/components/culprit_verification_table';
import { CulpritsTable } from '@/bisection/components/culprits_table/culprits_table';
import { HeuristicAnalysisTable } from '@/bisection/components/heuristic_analysis_table';
import { NthSectionAnalysisTable } from '@/bisection/components/nthsection_analysis_table';
import { useAnalysesClient } from '@/bisection/hooks/prpc_clients';
import {
  GenericCulpritWithDetails,
  GenericNthSectionAnalysisResult,
  GenericSuspect,
} from '@/bisection/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  Analysis,
  QueryAnalysisRequest,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

export interface TabPanelProps {
  readonly name: string;
  readonly value: string;
  readonly children?: React.ReactNode;
}

export function TabPanel({ value, name, children }: TabPanelProps) {
  return (
    <div hidden={value !== name} className="tab-panel">
      {value === name && <div className="tab-panel-contents">{children}</div>}
    </div>
  );
}

function getSuspects(analysis: Analysis): readonly GenericSuspect[] {
  const heuristicSuspects = analysis.heuristicResult?.suspects || [];
  const suspects = heuristicSuspects.map(GenericSuspect.fromHeuristic);
  const nthSectionSuspect = analysis.nthSectionResult?.suspect;
  if (nthSectionSuspect) {
    suspects.push(GenericSuspect.fromNthSection(nthSectionSuspect));
  }
  return suspects;
}

enum AnalysisComponentTabs {
  HEURISTIC = 'Heuristic analysis',
  NTH_SECTION = 'Nth section analysis',
  CULPRIT_VERIFICATION = 'Culprit verification',
}

export function AnalysisDetailsPage() {
  const { bbid } = useParams();
  if (!bbid) {
    // The page should always be mounted to a path where bbid is set.
    throw new Error('invariant violated: bbid should be set');
  }

  const [currentTab, setCurrentTab] = useState(AnalysisComponentTabs.HEURISTIC);

  const handleTabChange = (
    _: React.SyntheticEvent,
    newTab: AnalysisComponentTabs,
  ) => {
    setCurrentTab(newTab);
  };

  const client = useAnalysesClient();
  const { isLoading, isError, data, error } = useQuery(
    client.QueryAnalysis.query(
      QueryAnalysisRequest.fromPartial({
        buildFailure: {
          bbid: bbid,
          // TODO: update this once other failure types are analyzed
          failedStepName: 'compile',
        },
      }),
    ),
  );

  // TODO: display alert if the build ID queried is not the first failed build
  //       linked to the failure analysis

  if (isError) {
    return (
      <div className="section">
        <Alert severity="error">
          <AlertTitle>Failed to load analysis details</AlertTitle>
          {/* TODO: display more error detail for input issues e.g.
              Build not found, No analysis for that build, etc */}
          An error occurred when querying for the analysis details using build
          ID &quot;{bbid}&quot;:
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

  if (!data.analyses.length) {
    return <></>;
  }
  const analysis = data.analyses[0];

  return (
    <>
      <div className="section">
        <Typography variant="h5" gutterBottom>
          Analysis Details
        </Typography>
        <AnalysisOverview analysis={analysis} />
      </div>
      {analysis.culprits.length > 0 && (
        <div className="section">
          <Typography variant="h5" gutterBottom>
            Culprit Details
          </Typography>
          <CulpritsTable
            culprits={analysis.culprits.map(GenericCulpritWithDetails.from)}
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
            value={AnalysisComponentTabs.HEURISTIC}
            label={AnalysisComponentTabs.HEURISTIC}
          />
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
        <TabPanel value={currentTab} name={AnalysisComponentTabs.HEURISTIC}>
          <HeuristicAnalysisTable result={analysis.heuristicResult} />
        </TabPanel>
        <TabPanel value={currentTab} name={AnalysisComponentTabs.NTH_SECTION}>
          <NthSectionAnalysisTable
            result={
              analysis.nthSectionResult &&
              GenericNthSectionAnalysisResult.from(analysis.nthSectionResult)
            }
          />
        </TabPanel>
        <TabPanel
          value={currentTab}
          name={AnalysisComponentTabs.CULPRIT_VERIFICATION}
        >
          <CulpritVerificationTable suspects={getSuspects(analysis)} />
        </TabPanel>
      </div>
    </>
  );
}

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="analysis-details">
      <AnalysisDetailsPage />
    </RecoverableErrorBoundary>
  );
}
