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

import React from 'react';
import { Provider } from 'react-redux';
import {
  Route,
  Routes,
} from 'react-router-dom';

import BaseLayout from './src/layouts/base_layout';
import { store } from './src/store/store';
import Home from './src/views/home';

const App = () => {
  return (
    <Provider store={store}>
      <Routes>
        <Route path='/' element={<BaseLayout />}>
          <Route index element={<Home />} />
        </Route>
      </Routes>
    </Provider>
  );
};

export default App;
