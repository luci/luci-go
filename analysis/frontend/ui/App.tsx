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
import {
  QueryClient,
  QueryClientProvider,
} from 'react-query';
import {
  Route,
  Routes,
} from 'react-router-dom';

import FeedbackSnackbar from './src/components/error_snackbar/feedback_snackbar';
import { SnackbarContextWrapper } from './src/context/snackbar_context';
import BaseLayout from './src/layouts/base';
import ClusterPage from './src/views/clusters/cluster/cluster_page';
import ClustersPage from './src/views/clusters/clusters_page';
import NotFoundPage from './src/views/errors/not_found_page';
import HelpPage from './src/views/help/help_page';
import HomePage from './src/views/home/home_page';
import NewRulePage from './src/views/new_rule/new_rule';
import Rule from './src/views/rule/rule';
import RulesPage from './src/views/rules/rules_page';
import BugPage from './src/views/bug/bug_page/bug_page';

const queryClient = new QueryClient(
    {
      defaultOptions: {
        queries: {
          refetchOnWindowFocus: false,
        },
      },
    },
);

const App = () => {
  return (
    <SnackbarContextWrapper>
      <QueryClientProvider client={queryClient}>
        <Routes>
          <Route path='/' element={<BaseLayout />}>
            <Route index element={<HomePage />} />
            <Route path='help' element={<HelpPage />} />
            <Route path='b/:bugTracker/:id' element={<BugPage />} />
            <Route path='b/:id' element={<BugPage />} />
            <Route path='p/:project'>
              <Route path='rules'>
                <Route index element={<RulesPage />} />
                <Route path='new' element={<NewRulePage />} />
                <Route path=':id' element={<Rule />} />
              </Route>
              <Route path='clusters'>
                <Route index element={<ClustersPage />} />
                <Route path=':algorithm/:id' element={<ClusterPage />} />
              </Route>
            </Route>
            <Route path='*' element={<NotFoundPage />} />
          </Route>
        </Routes>
      </QueryClientProvider>
      <FeedbackSnackbar />
    </SnackbarContextWrapper>
  );
};

export default App;
