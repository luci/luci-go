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

import { Dispatch, SetStateAction, createContext, useState } from 'react';

import { UiPage } from '@/common/constants/view';

interface PageMetaContextData {
  selectedPage?: UiPage;
  setSelectedPage: Dispatch<SetStateAction<UiPage | undefined>>;
  project?: string;
  setProject: Dispatch<SetStateAction<string | undefined>>;
}

export const PageMetaContext = createContext<PageMetaContextData | null>(null);

interface Props {
  children: React.ReactNode;
  initProject?: string;
  initPage?: UiPage;
}

export const PageMetaProvider = ({
  children,
  initProject = '',
  initPage = UiPage.Builders,
}: Props) => {
  const [selectedPage, setSelectedPage] = useState<UiPage | undefined>(
    initPage,
  );
  const [project, setProject] = useState<string | undefined>(initProject);
  return (
    <PageMetaContext.Provider
      value={{
        selectedPage,
        setSelectedPage,
        project,
        setProject,
      }}
    >
      {children}
    </PageMetaContext.Provider>
  );
};
