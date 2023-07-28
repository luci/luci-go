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

import { useLoaderData } from 'react-router-dom';

import { AuthState } from '@/common/api/auth_state';
import { AuthStateProvider } from '@/common/components/auth_state_provider';
import { ErrorBoundary } from '@/common/components/error_boundary';
import { PageMetaProvider } from '@/common/components/page_meta/page_meta_provider';

import { MainContent } from './main_content/main_content';

/**
 * Renders page header, and tooltip.
 */
export function BaseLayout() {
  const initialAuthState = useLoaderData() as AuthState;

  return (
    <ErrorBoundary>
      <AuthStateProvider initialValue={initialAuthState}>
        <PageMetaProvider>
          <MainContent />
        </PageMetaProvider>
      </AuthStateProvider>
    </ErrorBoundary>
  );
}
