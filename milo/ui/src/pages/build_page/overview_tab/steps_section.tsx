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

import { observer } from 'mobx-react-lite';

import { useStore } from '@/common/store';
import { getBuildURLPath } from '@/common/tools/url_utils';

import { StepDisplayConfig } from '../steps_tab/step_display_config';
import { StepList } from '../steps_tab/step_list';

export const StepsSection = observer(() => {
  const store = useStore();
  const pageState = store.buildPage;

  if (!pageState.builderIdParam || !pageState.buildNumOrIdParam) {
    return <></>;
  }

  const stepsUrl =
    getBuildURLPath(pageState.builderIdParam, pageState.buildNumOrIdParam) +
    '/steps';

  return (
    <>
      <h3>
        Steps & Logs (<a href={stepsUrl}>View in Steps Tab</a>)
      </h3>
      <StepDisplayConfig />
      <StepList />
    </>
  );
});
