// Copyright 2022 The LUCI Authors.
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


import './styles/style.css';

import * as React from 'react';
import { QueryClient, QueryClientProvider } from 'react-query';
import { Route, Routes } from 'react-router-dom';

import { BaseLayout } from './src/layouts/base';
import { AnalysisDetailsPage } from './src/views/analysis_details/analysis_details';
import { FailureAnalysesPage } from './src/views/failure_analyses';
import { NotFoundPage } from './src/views/not_found';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
    },
  },
});

export const App = () => {
  return (
    <QueryClientProvider client={queryClient}>
      <Routes>
        <Route path='/' element={<BaseLayout />}>
          <Route index element={<FailureAnalysesPage />} />
          <Route path='analysis/b/:bbid' element={<AnalysisDetailsPage />} />
          <Route path='*' element={<NotFoundPage />} />
        </Route>
      </Routes>
    </QueryClientProvider>
  );
};
