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

import { useEffect } from 'react';
import { Helmet } from 'react-helmet';

import milo from '@/common/assets/favicons/milo-32.png';
import { UiPage } from '@/common/constants/view';

import { useProject, useSetProject, useSetSelectedPage } from './hooks';

const DEFAULT_FAVICON = milo;

export interface PageMetaProps {
  title: string;
  selectedPage?: UiPage;
  project?: string;
  favicon?: string;
}

/**
 * Handles setting the sidebar selected item and title of the page.
 */
export const PageMeta = ({
  title,
  selectedPage,
  project,
  favicon,
}: PageMetaProps) => {
  const setProject = useSetProject();
  const setSelectedPage = useSetSelectedPage();
  const currentProject = useProject();

  useEffect(() => {
    setSelectedPage(selectedPage);
    setProject(project || currentProject);
  }, [currentProject, project, selectedPage, setProject, setSelectedPage]);

  useEffect(() => {
    if (favicon === undefined) {
      return () => {};
    }

    document.getElementById('favicon')?.setAttribute('href', favicon);
    return () => {
      document.getElementById('favicon')?.setAttribute('href', DEFAULT_FAVICON);
    };
  }, [favicon]);

  return (
    <Helmet titleTemplate="%s | LUCI" defaultTitle="LUCI">
      {title && <title>{title}</title>}
    </Helmet>
  );
};
