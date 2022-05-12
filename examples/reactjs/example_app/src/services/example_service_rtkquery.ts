// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react';
import { ExampleModel } from './example_service';

// Define a service using a base URL and expected endpoints
export const exampleApi = createApi({
  reducerPath: 'exxampleApi',
  baseQuery: fetchBaseQuery({ baseUrl: '/api/v2/' }),
  endpoints: (builder) => ({
    getExampleByName: builder.query<ExampleModel, string>({
      query: (name) => `user/${name}`,
    }),
  }),
});

// Export hooks for usage in functional components, which are
// auto-generated based on the defined endpoints
export const { useGetExampleByNameQuery } = exampleApi;
