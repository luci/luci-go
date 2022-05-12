// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
