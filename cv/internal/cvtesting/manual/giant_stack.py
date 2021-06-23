#!/usr/bin/env python3
# Copyright 2021 The LUCI Authors.
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

import argparse
import sys
import hashlib
import os
import contextlib
import json
import subprocess


KNOWN_REPOS = {
    'public': 'https://chromium.googlesource.com/playground/gerrit-cq',
    'internal': 'https://chrome-internal.googlesource.com/playground/cq',
}


def parse_args():
  p = argparse.ArgumentParser()
  p.add_argument('-n', '--number', type=int, required=True,
      help='CLs number')
  p.add_argument('--cq', type=int, required=True,
      help='+1 or +2 to vote on CQ label')
  p.add_argument('-w', '--work-dir', dest='workdir', required=True,
      help='Path where repos are checked out and results are stored')
  p.add_argument('--id',
      help='ID of this test. Defaults to current timestamp')
  p.add_argument('--description', default='UNSPECIFIED',
      help='Description of this test, should fit on one line')
  opts = p.parse_args()
  if not opts.id:
    opts.id = datetime.datetime.now().isoformat()
    opts.id = opts.id[:opts.id.rfind('.')] # strip fractional secs
  if ' ' in opts.id:
    p.error('spaces not allowed in --id')
  return opts


def main():
  opts = parse_args()
  ensure_work_dir(opts)
  checkout_repos(opts)
  with recorder(os.path.join(opts.workdir, opts.id)) as rec:
    cls = create_cls(opts, rec, kind='internal')
    cq(opts, cls, rec)


@contextlib.contextmanager
def recorder(fpath):
  records = 0
  with open(fpath, 'wb') as f:
    logging.info('Recording actions in %r as Line-delimited JSON', fpath)

    def dump(**obj):
      json.dump(obj, f)
      f.write('\n')
      f.flush()
      records += 1

    class Recorder(object):
      def create_cl(self, host, number, repo):
        dump(action='create_cl', payload=dict(host=host, number=number, repo=repo))
      def cq(self, host, number, value):
        dump(action='cq', payload=dict(host=host, number=number, value=value))

    yield Recorder()
  logging.info('Wrote %d records in %r as Line-delimited JSON', records, fpath)


def ensure_work_dir(opts):
  if not os.path.exists(opts.workdir):
    logging.info('Creating %r workdir', opts.workdir)
    os.makedirs(opts.workdir)
    return
  if os.path.isdir(opts.workdir):
    return
  raise Exception('%s is not a dir' % opts.workdir)


def checkout_repos(opts):
  for kind, origin in KNOWN_REPOS.iteritems():
    if os.path.exists(os.path.join(cwd, kind)):
      continue
    git('clone', origin, kind, cwd=opts.workdir)


def create_cls(opts, rec, kind):
  assert kind in KNOWN_REPOS, (kind, KNOWN_REPOS)
  cwd = os.path.join(opts.workdir, kind)
  git('freeze', cwd=cwd)  # just in case
  git('fetch', 'origin', cwd=cwd)  # avoid conflicts

  hash_prefix = hashlib.sha1(opts.id).hexdigest()[:-4]
  git('new-branch', '%s-%s' % (opts.id, hash_prefix), cwd=cwd)
  os.mkdir(os.path.join(cwd, opts.id))
  for i in range(opts.number):
    gen_commit(opts, n=i+1, rel_parent_dir=opts.id, hash_prefix=hash_prefix, cwd=cwd)


def gen_commit(opts, n, rel_parent_dir, hash_prefix, cwd):
  rel_fpath = os.path.join(rel_parent_dir, '%03d' % n)
  # Full sha1 hash is 40 chars long.
  suffix_tmpl '%%0%dd' % (40-len(hash_prefix),)
  change_id = hash_prefix + (suffix_tmpl % (n,))
  with open(os.path.join(cwd, rel_fpath), 'w') as f:
    f.write('Commit #%d\n' % n)
  git('add', rel_fpath, cwd=cwd)
  msg_lines = [
      'Commit #%d' % n,
      '',
      'Test-Description: %s' % opts.description,
      'Test-Id: %s' % opts.id,
      'Change-Id: I%s' % change_id,
  ]
  msg = '\n'.join(msg_lines)
  git('commit', '-m', msg, cwd=cwd)


def git(*args, cwd=None):
  out = subprocess.check_output(['git'] + list(args), cwd=cwd)
  return out.strip()


if __name__ == '__main__':
  main()
  sys.exit(0)
