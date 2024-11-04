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

import { Link, Outlet } from 'react-router-dom';

export const FleetLayout = () => {
  return (
    <div>
      <header
        css={{
          display: 'flex',
          flexDirection: 'row',
          gap: 20,
          alignItems: 'center',
          backgroundColor: 'white',
          border: 'solid black 1px',
          margin: 5,
          padding: 5,
        }}
      >
        <Link to="/ui">Home</Link>
        <p>Fleet custom layout</p>
      </header>
      <Outlet />
      {/* TODO add privacy footer and cookie consent bar as per org policy */}
    </div>
  );
};
