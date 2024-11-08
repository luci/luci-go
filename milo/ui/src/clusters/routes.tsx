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

import { Rule } from '@mui/icons-material';
import { Route, Routes } from 'react-router-dom';

import FeedbackSnackbar from './components/error_snackbar/feedback_snackbar';
import { SnackbarContextWrapper } from './context/snackbar_context';
import BugPage from './views/bug/bug_page/bug_page';
import ClusterPage from './views/clusters/cluster/cluster_page';
import ClustersPage from './views/clusters/clusters_page';
import HelpPage from './views/help/help_page';
import NewRulePage from './views/new_rule/new_rule';
import RulesPage from './views/rules/rules_page';

export function ClustersRoutes() {
  return (
    <SnackbarContextWrapper>
      <Routes>
        <Route path="help" element={<HelpPage />} />
        <Route path="b/:bugTracker/:id" element={<BugPage />} />
        <Route path="b/:id" element={<BugPage />} />
        <Route path="p/:project">
          <Route path="rules">
            <Route index element={<RulesPage />} />
            <Route path="new" element={<NewRulePage />} />
            <Route path=":id" element={<Rule />} />
          </Route>
          <Route path="clusters">
            <Route index element={<ClustersPage />} />
            <Route path=":algorithm/:id" element={<ClusterPage />} />
          </Route>
        </Route>
      </Routes>
      <FeedbackSnackbar />
    </SnackbarContextWrapper>
  );
}
