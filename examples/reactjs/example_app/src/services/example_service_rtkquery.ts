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

import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react';
import { ExampleModel } from '@/services/example_service';

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
