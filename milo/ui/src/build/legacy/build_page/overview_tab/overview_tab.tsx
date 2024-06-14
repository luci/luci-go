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

import { Box, styled } from '@mui/material';

import { BuildActionButton } from '@/build/components/build_action_button';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useTabId } from '@/generic_libs/components/routed_tabs';

import { useBuild } from '../context';
import { AlertsSection } from '../infra_tab/alerts_section';
import { FailedTestSection } from '../infra_tab/failed_tests_section';
import { StepsSection } from '../infra_tab/steps_section';
import { SummarySection } from '../infra_tab/summary_section';

import { BuildDescription } from './description_section';

const ContainerDiv = styled(Box)({
  padding: '5px 16px',
  '& h3': {
    marginBlock: '25px 10px',
  },
  '& h3:first-of-type': {
    marginTop: '10px',
  },
});

export function OverviewTab() {
  const build = useBuild();

  return (
    <ContainerDiv>
      <BuildDescription />
      {build && <BuildActionButton build={build} />}
      <AlertsSection />
      <SummarySection />
      <FailedTestSection />
      <StepsSection />
    </ContainerDiv>
  );
}

export function Component() {
  useTabId('overview');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="overview">
      <OverviewTab />
    </RecoverableErrorBoundary>
  );
}
