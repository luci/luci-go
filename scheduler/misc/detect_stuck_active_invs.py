#!/usr/bin/env python
# Copyright 2018 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Finds jobs with old entries (>1d) in ActiveInvocations list.

Usage:
  prpc login
  ./detect_stuck_active_invs.py luci-scheduler-dev.appspot.com

Requires the caller to be in 'administrators' group.
"""

import json
import subprocess
import sys
import time


def prpc(host, method, body):
  p = subprocess.Popen(
      ['prpc', 'call', host, method],
      stdin=subprocess.PIPE,
      stdout=subprocess.PIPE)
  out, _ = p.communicate(json.dumps(body))
  if p.returncode:
    raise Exception('Call to %s failed' % method)
  return json.loads(out)


def check_job(host, job_ref):
  print 'Checking %s/%s' % (job_ref['project'], job_ref['job'])

  state = prpc(host, 'internal.admin.Admin.GetDebugJobState', job_ref)
  active_invs = state.get('activeInvocations', [])
  if not active_invs:
    print '  No active invocations'
    return []

  stuck = []
  for inv_id in active_invs:
    print '  ...checking %s' % inv_id
    inv = prpc(host, 'scheduler.Scheduler.GetInvocation', {
      'jobRef': job_ref,
      'invocationId': inv_id,
    })
    started = time.time() - int(inv['startedTs']) / 1000000.0
    if started > 24 * 3600:
      print '    it is stuck!'
      stuck.append((job_ref, inv_id))
  return stuck


def main():
  if len(sys.argv) != 2:
    print >> sys.stderr, 'Usage: %s <host>' % sys.argv[0]
    return 1
  host = sys.argv[1]

  stuck = []
  for job in prpc(host, 'scheduler.Scheduler.GetJobs', {})['jobs']:
    stuck.extend(check_job(host, job['jobRef']))

  if not stuck:
    print 'No invocations are stuck'
    return

  print
  print 'All stuck invocations: '
  for job_ref, inv_id in stuck:
    print '%s/%s %s' % (job_ref['project'], job_ref['job'], inv_id)

  return 0


if __name__ == '__main__':
  sys.exit(main())
