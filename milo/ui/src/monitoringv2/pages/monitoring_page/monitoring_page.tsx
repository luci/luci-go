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

import NotesIcon from '@mui/icons-material/Notes';
import {
  Avatar,
  List,
  ListItemAvatar,
  ListItemButton,
  ListItemText,
  Typography,
} from '@mui/material';
import { forwardRef } from 'react';
import {
  Link as RouterLink,
  LinkProps as RouterLinkProps,
  useParams,
} from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta, usePageId, useProject } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { Alerts } from '@/monitoringv2/components/alerts';
import { configuredTrees } from '@/monitoringv2/util/config';

import { MonitoringProvider } from './context';

export const MonitoringPage = () => {
  const { tree: treeName } = useParams();
  const tree = configuredTrees.filter((t) => t.name === treeName).at(0);
  useProject(tree?.project);

  return (
    <MonitoringProvider tree={tree} treeName={treeName}>
      <PageMeta title="Monitoring" />
      {!tree || !treeName ? (
        <>
          <Typography
            sx={{
              paddingLeft: 1,
            }}
            variant="h4"
          >
            Monitoring: Trees
          </Typography>
          <List
            component="nav"
            sx={{ width: '100%', maxWidth: 360, bgcolor: 'background.paper' }}
          >
            {configuredTrees.map((t) => (
              <ListItemButton
                key={t.display_name}
                component={Link}
                to={`/ui/labs/monitoring/${t.name}`}
              >
                <ListItemAvatar>
                  <Avatar>
                    <NotesIcon />
                  </Avatar>
                </ListItemAvatar>
                <ListItemText primary={t.display_name} secondary={t.name} />
              </ListItemButton>
            ))}
          </List>
        </>
      ) : (
        <Alerts />
      )}
    </MonitoringProvider>
  );
};

const Link = forwardRef<HTMLAnchorElement, RouterLinkProps>(
  function Link(itemProps, ref) {
    return <RouterLink ref={ref} {...itemProps} role={undefined} />;
  },
);

export function Component() {
  usePageId(UiPage.Monitoring);

  return (
    <TrackLeafRoutePageView contentGroup="monitoring-v2">
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error this
        // way.
        key="monitoring-page-v2"
      >
        <MonitoringPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
