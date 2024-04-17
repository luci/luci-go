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

import styled from '@emotion/styled';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useTabId } from '@/generic_libs/components/routed_tabs';

import { AlertsSection } from './alerts_section';
import { BuildActionButton } from './build_action_button';
import { BuildDescription } from './description_section';
import { FailedTestSection } from './failed_tests_section';
import { StepsSection } from './steps_section';
import { SummarySection } from './summary_section';

const ContainerDiv = styled.div({
  margin: '10px 16px',
  '& h3': {
    marginBlock: '25px 10px',
  },
  '& h3:first-of-type': {
    marginTop: '10px',
  },
});

export function OverviewTab() {
  return (
    <ContainerDiv>
      <BuildDescription />
      <BuildActionButton />
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
