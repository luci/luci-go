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

import {
  Route,
  Routes,
  Navigate,
} from 'react-router-dom';

import { GlobalsProvider } from './context/globals';

import Layout from './layout';
import NotFound from './views/not_found';
import ServicesList from './views/services_list';
import MethodsList from './views/methods_list';
import Method from './views/method';

const App = () => {
  return (
    <GlobalsProvider>
      <Routes>
        <Route path='/' element={<Layout />}>
          <Route index element={<Navigate replace to='/services/' />} />
          <Route path='services'>
            <Route index element={<ServicesList />} />
            <Route path=':serviceName'>
              <Route index element={<MethodsList />} />
              <Route path=':methodName' element={<Method />} />
            </Route>
          </Route>
          <Route path='*' element={<NotFound />} />
        </Route>
      </Routes>
    </GlobalsProvider>
  );
};

export default App;
