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
import { observer } from 'mobx-react-lite';
import { useEffect, useState } from 'react';

import { useStore } from '@/common/store';

import { ActionsSection, Dialog } from './actions_section';
import { BuildLogSection } from './build_log_section';
import { BuildPackagesInfoSection } from './build_packages_info_section';
import { BuilderInfoSection } from './builder_info_section';
import { CancelBuildDialog } from './cancel_build_dialog';
import { ExperimentsSection } from './experiments_section';
import { FailedTestSection } from './failed_tests_section';
import { InfraSection } from './infra_section';
import { InputSection } from './input_section';
import { OutputSection } from './output_section';
import { PropertiesSection } from './properties_section';
import { RetryBuildDialog } from './retry_build_dialog';
import { StepsSection } from './steps_section';
import { SummarySection } from './summary_section';
import { TagsSection } from './tags_section';
import { TimingSection } from './timing_section';

const ContainerDiv = styled.div({
  margin: '10px 16px',
  '@media screen and (min-width: 1300px)': {
    display: 'grid',
    gridTemplateColumns: '1fr 20px 40vw',
  },
  '& h3': {
    marginBlock: '15px 10px',
  },
  '& h4': {
    marginBlock: '10px 10px',
  },
  '& td:nth-of-type(2)': {
    clear: 'both',
    overflowWrap: 'anywhere',
  },
  '& h3:first-child': {
    marginBlock: '5px 10px',
  },
});

const FirstColumn = styled.div({
  overflow: 'hidden',
  gridColumn: 1,
});

const SecondColumn = styled.div({
  overflow: 'hidden',
  gridColumn: 3,
});

export const OverviewTab = observer(() => {
  const store = useStore();

  const [activeDialog, setActiveDialog] = useState(Dialog.None);

  useEffect(() => {
    store.setSelectedTabId('overview');
  }, [store]);

  return (
    <>
      <RetryBuildDialog
        open={activeDialog === Dialog.RetryBuild}
        onClose={() => setActiveDialog(Dialog.None)}
      />
      <CancelBuildDialog
        open={activeDialog === Dialog.CancelBuild}
        onClose={() => setActiveDialog(Dialog.None)}
      />
      <ContainerDiv>
        <FirstColumn>
          <SummarySection />
          <FailedTestSection />
          <StepsSection />
        </FirstColumn>
        <SecondColumn>
          {store.buildPage.build?.data.builder && (
            <BuilderInfoSection
              builderId={store.buildPage.build.data.builder}
            />
          )}
          <InputSection />
          <OutputSection />
          <InfraSection />
          <TimingSection />
          <BuildLogSection />
          <ActionsSection openDialog={(dialog) => setActiveDialog(dialog)} />
          <TagsSection />
          <ExperimentsSection />
          <PropertiesSection />
          <BuildPackagesInfoSection />
        </SecondColumn>
      </ContainerDiv>
    </>
  );
});
