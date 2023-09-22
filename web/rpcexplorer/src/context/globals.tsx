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
  createContext,
  useContext, useEffect, useState,
} from 'react';

import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';

import { ErrorAlert } from '../components/error_alert';

import { loadOAuthClient, OAuthClient } from '../data/oauth';
import { Descriptors, loadDescriptors } from '../data/prpc';


// Globals are fetched once and then indefinitely used by all routes.
export interface Globals {
  // Type information with server's RPC APIs.
  descriptors: Descriptors;
  // The OAuth client to use for getting an access token.
  oauthClient: OAuthClient;
}

// loadGlobals loads the globals by querying the server.
const loadGlobals = async (): Promise<Globals> => {
  const [descriptors, oauthClient] = await Promise.all([
    loadDescriptors(),
    loadOAuthClient(),
  ]);
  return {
    descriptors: descriptors,
    oauthClient: oauthClient,
  };
};


// GlobalsContextData wraps Globals with loading status.
interface GlobalsContextData {
  isLoading: boolean;
  error?: Error;
  globals?: Globals;
}

const GlobalsContext = createContext<GlobalsContextData>({
  isLoading: true,
});


export interface GlobalsProviderProps {
  children: React.ReactNode;
}

// GlobalsProvider loads globals and makes them accessible to children elements.
export const GlobalsProvider = ({ children }: GlobalsProviderProps) => {
  const [globalsData, setGlobalsData] = useState<GlobalsContextData>({
    isLoading: true,
  });

  useEffect(() => {
    loadGlobals()
        .then((globals) => setGlobalsData({
          isLoading: false,
          globals: globals,
        }))
        .catch((error) => setGlobalsData({
          isLoading: false,
          error: error,
        }));
  }, []);

  return (
    <GlobalsContext.Provider value={globalsData}>
      {children}
    </GlobalsContext.Provider>
  );
};


export interface GlobalsWaiterProps {
  children: React.ReactNode;
}

// GlobalsWaiter waits for globals to be loaded.
//
// On success, it renders the children. On error it renders the error message.
export const GlobalsWaiter = ({ children }: GlobalsWaiterProps) => {
  const globalsData = useContext(GlobalsContext);

  if (globalsData.isLoading) {
    return (
      <Box sx={{ width: '100%' }}>
        <LinearProgress />
      </Box>
    );
  }

  if (globalsData.error) {
    return (
      <ErrorAlert
        title="Failed to initialize RPC Explorer"
        error={globalsData.error}
      />
    );
  }

  return <>{children}</>;
};


// useGlobals returns loaded globals or throws an error.
//
// Should be used somewhere under GlobalsWaiter component, which renders
// children only if globals are actually loaded.
export const useGlobals = (): Globals => {
  const globalsData = useContext(GlobalsContext);
  if (globalsData.isLoading) {
    throw new Error('Globals are not loaded yet');
  }
  if (globalsData.error) {
    throw new Error('Globals failed to be loaded');
  }
  if (globalsData.globals === undefined) {
    throw new Error('Globals unexpectedly empty');
  }
  return globalsData.globals;
};
