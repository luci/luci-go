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

import { Box, styled } from '@mui/material';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useTabId } from '@/generic_libs/components/routed_tabs';

import { useBuild } from '../context';

import { AlertsSection } from './alerts_section';
import { BuildLogSection } from './build_log_section';
import { BuildPackagesInfoSection } from './build_packages_info_section';
import { BuilderInfoSection } from './builder_info_section';
import { ExperimentsSection } from './experiments_section';
import { FailedTestSection } from './failed_tests_section';
import { InfraSection } from './infra_section';
import { InputSection } from './input_section';
import { OutputSection } from './output_section';
import { PropertiesSection } from './properties_section';
import { StepsSection } from './steps_section';
import { SummarySection } from './summary_section';
import { TagsSection } from './tags_section';
import { TimingSection } from './timing_section';

const ContainerDiv = styled(Box)({
  padding: '5px 16px',
  '@media screen and (min-width: 1300px)': {
    display: 'grid',
    gridTemplateColumns: '1fr 40vw',
    gap: '20px',
  },
  '& h3': {
    marginBlock: '25px 10px',
  },
  '& h3:first-of-type': {
    marginTop: '10px',
  },
});

export function InfraTab() {
  const build = useBuild();

  if (!build) {
    return;
  }

  return (
    <ContainerDiv>
      <Box>
        <AlertsSection />
        <SummarySection />
        <FailedTestSection />
        <StepsSection />
      </Box>
      <Box>
        {build.builderInfo?.description && (
          <BuilderInfoSection descriptionHtml={build.builderInfo.description} />
        )}
        <InputSection />
        <OutputSection />
        <InfraSection />
        <TimingSection />
        <BuildLogSection />
        <TagsSection />
        <ExperimentsSection />
        <PropertiesSection />
        <BuildPackagesInfoSection />
      </Box>
    </ContainerDiv>
  );
}

export function Component() {
  useTabId('infra');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="infra">
      <InfraTab />
    </RecoverableErrorBoundary>
  );
}
