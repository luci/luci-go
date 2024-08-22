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

import { useContext } from 'react';

import { PageMetaContext } from './page_meta_provider';

export function useSelectedPage() {
  const context = useContext(PageMetaContext);

  if (!context) {
    throw new Error('useSelectedPage can only be used in a PageMetaContext');
  }

  return context.selectedPage;
}

export function useProject() {
  const context = useContext(PageMetaContext);

  if (!context) {
    throw new Error('useSelectedProject can only be used in a PageMetaContext');
  }

  return context.project;
}

export function useSetSelectedPage() {
  const context = useContext(PageMetaContext);

  if (!context) {
    throw new Error('useSetSelectedPage can only be used in a PageMetaContext');
  }

  return context.setSelectedPage;
}

export function useSetProject() {
  const context = useContext(PageMetaContext);

  if (!context) {
    throw new Error('useSetProject can only be used in a PageMetaContext');
  }

  return context.setProject;
}
