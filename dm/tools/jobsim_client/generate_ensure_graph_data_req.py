#!/usr/bin/env python
# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

"""
This is a dumb script that will:
  * compile jobsim_client (for linux-amd64)
  * upload a simple cipd package containing only jobsim_client with the 'latest'
    ref. The package name is 'infra/experimental/dm/jobsim_client/linux-amd64'.
  * Print a JSONPB-encoded EnsureGraphDataReq that runs jobsim_client with the
    provided strings to calculate edit-distance(a, b, transposition?).
"""

import argparse
import json
import os
import pprint
import shutil
import subprocess
import tempfile

THIS_DIR = os.path.dirname(os.path.abspath(__file__))

def compile_pkg(pkg_dir, os_name, arch_name):
  print 'building jobsim_client'
  env = os.environ.copy()
  env.update(
    GOOS   = os_name,
    GOARCH = arch_name,
  )
  subprocess.check_call(
      ['go', 'build', 'go.chromium.org/luci/dm/tools/jobsim_client'],
      cwd=pkg_dir, env=env)

def upload_pkg(pkg_dir, pkg_name_prefix, os_name, arch_name):
  print 'creating jobsim_client package'

  pkg_name = '%s/%s-%s' % (pkg_name_prefix, os_name, arch_name)

  fd, tfile = tempfile.mkstemp('-cipd-output.json')
  os.close(fd)
  try:
    subprocess.check_call(['cipd', 'create', '-name', pkg_name,
                           '-ref', 'latest', '-in', pkg_dir,
                           '-install-mode', 'copy', '-json-output', tfile])
    with open(tfile, 'r') as tfileData:
      out_json = json.load(tfileData)
    version = out_json[u'result'][u'instance_id'].encode('utf-8')
  finally:
    os.unlink(tfile)

  print 'uploaded %s:%s' % (pkg_name, version)

  return pkg_name, version

def print_req(opts, pkg_name, version):
  def dumps(obj):
    return json.dumps(obj, sort_keys=True, separators=(',', ':'))

  cpu = {
    'amd64': 'x86-64',
  }[opts.arch]

  os_name = {
    'linux': 'Linux',
  }[opts.os]

  command = ['jobsim_client', 'edit-distance', '-dm-host', '${DM.HOST}',
             '-quest-desc-path', '${DM.QUEST.DATA.DESC:PATH}']
  if opts.use_transposition:
    command.append('-use-transposition')

  distParams = {
    'scheduling': {
      'dimensions': {
        'cpu': cpu,
        'os': os_name,
        'pool': opts.pool,
      },
    },
    'meta': {'name_prefix': 'dm jobsim client'},
    'job': {
      'inputs': {
        'cipd': {
          'server': 'https://chrome-infra-packages.appspot.com',
          'by_path': {
            '.': {
              'pkg': [
                {
                  'name': pkg_name,
                  'version': version if opts.pin else 'latest',
                },
              ]
            }
          }
        }
      },
      'command': command,
    }
  }

  if opts.snapshot_dimension:
    distParams['scheduling']['snapshot_dimensions'] = opts.snapshot_dimension

  params = {
    'a': opts.a,
    'b': opts.b,
  }

  desc = {
    'quest': [
      {
        'distributor_config_name': 'swarming',
        'parameters': dumps(params),
        'distributor_parameters': dumps(distParams),
        'meta': {
          'timeouts': {
            'start': '600s',
            'run': '300s',
            'stop': '300s',
          }
        },
      }
    ],
    'quest_attempt': [
      {'nums': [1]},
    ]
  }

  print dumps(desc)

def main():
  parser = argparse.ArgumentParser(
      description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
  parser.add_argument('--use-transposition', action='store_true', default=False,
                      help=('Use Damerau-Levenshtein distance calculation '
                            'instead of plain Levenshtein distance.'))
  parser.add_argument('a', type=str, help='The "a" string to calculate for.')
  parser.add_argument('b', type=str, help='The "b" string to calculate for.')

  plat_grp = parser.add_argument_group(
      'platform', 'Options for the target platform of the job.')
  plat_grp.add_argument('--os', choices=('linux',), default='linux',
                        help='The OS to compile/run on.')
  plat_grp.add_argument('--arch', choices=('amd64',), default='amd64',
                        help='The Arch to compile/run on.')
  plat_grp.add_argument('--pool', type=str, default='default',
                        help='The swarming pool to use.')
  plat_grp.add_argument('--pin', action='store_true', default=False,
                        help='Emit the request with a pinned package version'
                        ' instead of "latest".')
  plat_grp.add_argument(
    '--snapshot-dimension', action='append', help=(
      'Pin this dimension on re-executions. This will cause re-executions to '
      'copy the most-specific value of this dependency from the first '
      'execution to all subsequent re-executions of the same Attempt. May be '
      'specified multiple times.'
    ))

  cipd_grp = parser.add_argument_group('cipd', 'cipd packaging options')
  cipd_grp.add_argument('--cipd-service-url', default=None,
                        help='The CIPD service to upload to.')
  cipd_grp.add_argument('--cipd-service-account-json', default=None,
                        help='The CIPD service account JSON file to use.')
  cipd_grp.add_argument('--cipd-name',
                        default='infra/experimental/dm/jobsim_client',
                        help='The CIPD package name prefix to upload to. This '
                        'will be appended with the standard os-arch suffix.')

  opts = parser.parse_args()

  # Use local path for determinisim.
  pkg_dir = os.path.join(THIS_DIR, 'pkg_dir')
  shutil.rmtree(pkg_dir, ignore_errors=True)
  os.mkdir(pkg_dir)
  try:
    compile_pkg(pkg_dir, opts.os, opts.arch)
    pkg_name, version = upload_pkg(pkg_dir, opts.cipd_name, opts.os, opts.arch)
    print_req(opts, pkg_name, version)
  finally:
    shutil.rmtree(pkg_dir, ignore_errors=True)


if __name__ == '__main__':
  main()
