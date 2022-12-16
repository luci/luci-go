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
  useContext,
  useState,
  useEffect,
} from 'react';

import { Descriptors, loadDescriptors } from '../data/prpc';
import { loadOAuthClientId } from '../data/oauth';

// Globals are fetched once and then indefinitely used by all routes.
export interface Globals {
  // Type information with server's RPC APIs.
  descriptors: Descriptors;
  // The OAuth client ID to use for making authenticated RPC calls.
  oauthClientId: string;
}

// loadGlobals loads the globals by querying the server.
const loadGlobals = async (): Promise<Globals> => {
  const [descriptors, oauthClientId] = await Promise.all([
    loadDescriptors(),
    loadOAuthClientId(),
  ]);
  return {
    descriptors: descriptors,
    oauthClientId: oauthClientId,
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
      <p>Loading...</p>
    );
  }

  if (globalsData.error) {
    return (
      <p>Error: {globalsData.error.message}</p>
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
