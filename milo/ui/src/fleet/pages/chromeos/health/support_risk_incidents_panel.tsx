// Copyright 2026 The LUCI Authors.
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
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Divider,
  List,
  ListItem,
  Skeleton,
  Typography,
} from '@mui/material';

import { useSupportRiskIncidents } from './use_support_risk_incidents';

export interface SupportRiskIncidentsPanelProps {
  onShowModel?: (model: string) => void;
}

export const SupportRiskIncidentsPanel = ({
  onShowModel,
}: SupportRiskIncidentsPanelProps) => {
  const { data, isLoading, isError } = useSupportRiskIncidents();
  const incidents = data?.incidents ?? [];

  return (
    <Card
      variant="outlined"
      sx={{
        display: 'flex',
        flexDirection: 'column',
        width: '100%',
        maxHeight: { xs: 400, md: 520 },
        bgcolor: 'background.paper',
        borderRadius: 2,
      }}
    >
      <CardHeader
        title={
          <Typography component="h2" sx={{ fontWeight: 'bold', fontSize: 15 }}>
            Active Support Risk Incidents
          </Typography>
        }
      />
      <Divider />
      <CardContent
        sx={{
          p: 0,
          flexGrow: 1,
          display: 'flex',
          flexDirection: 'column',
          minHeight: 120,
          overflowY: 'auto',
          '&:last-child': { pb: 0 },
        }}
      >
        {isLoading && (
          <List dense sx={{ p: 0 }} aria-label="Loading incidents">
            {[1, 2, 3].map((key) => (
              <ListItem
                key={key}
                sx={{
                  py: 1.25,
                  px: 2,
                  borderBottom: '1px solid',
                  borderColor: 'divider',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  '&:last-child': { borderBottom: 'none' },
                }}
              >
                <Box sx={{ minWidth: 0, flex: 1, mr: 1 }}>
                  <Skeleton
                    variant="text"
                    width={key === 1 ? '75%' : key === 2 ? '60%' : '80%'}
                    height={20}
                  />
                  <Skeleton
                    variant="text"
                    width="35%"
                    height={16}
                    sx={{ mt: 0.25 }}
                  />
                </Box>
                <Box sx={{ flexShrink: 0 }}>
                  <Skeleton
                    variant="rounded"
                    width={52}
                    height={24}
                    sx={{ borderRadius: 1 }}
                  />
                </Box>
              </ListItem>
            ))}
          </List>
        )}

        {isError && (
          <Box p={2}>
            <Alert severity="error">
              Failed to load support risk incidents.
            </Alert>
          </Box>
        )}

        {!isLoading && !isError && incidents.length === 0 && (
          <Box
            display="flex"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            py={4}
            px={2}
          >
            <Typography variant="body2" color="text.secondary">
              No active Support Risk incidents.
            </Typography>
          </Box>
        )}

        {!isLoading && !isError && incidents.length > 0 && (
          <List dense sx={{ p: 0 }}>
            {incidents.map((incident) => (
              <ListItem
                key={incident.id}
                sx={{
                  py: 1.25,
                  px: 2,
                  borderBottom: '1px solid',
                  borderColor: 'divider',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  '&:hover': { bgcolor: 'action.hover' },
                  '&:last-child': { borderBottom: 'none' },
                }}
              >
                <Box sx={{ minWidth: 0, flex: 1, mr: 1 }}>
                  <Typography
                    variant="body2"
                    title={`b/${incident.buganizerId} - ${incident.title}`}
                    sx={{
                      fontWeight: 'bold',
                      color: 'primary.main',
                      fontSize: 13,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    b/{incident.buganizerId} - {incident.title}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block', fontSize: 12, mt: 0.25 }}
                  >
                    Model: <b>{incident.model}</b>
                  </Typography>
                </Box>
                <Box
                  sx={{
                    display: 'flex',
                    gap: 0.5,
                    alignItems: 'center',
                    flexShrink: 0,
                  }}
                >
                  {/* The Show button is currently a no-op placeholder callback,
                      but in future iterations its role will be to apply a filter
                      to only show that model in the dashboard/devices list */}
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={() => onShowModel?.(incident.model)}
                    sx={{
                      textTransform: 'none',
                      fontSize: 11,
                      py: 0.2,
                      px: 1.2,
                    }}
                  >
                    Show
                  </Button>
                </Box>
              </ListItem>
            ))}
          </List>
        )}
      </CardContent>
    </Card>
  );
};
