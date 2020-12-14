from collections import deque
import argparse
import json
import os
import shutil
import subprocess
import sys


def bin_suffix():
  if sys.platform == 'win32':
    return '.exe'
  if sys.platform == 'darwin':
    return '_mac'
  return ''


def download_isolated_tests(isolateserver, isolated_tests):
  for test, isolated_hash in isolated_tests.items():
    download_isolated_files(isolateserver, test, isolated_hash)
    create_isolate_file(test)
    create_isolated_gen_file(test)


def download_isolated_files(isolateserver, test, isolated_hash):
  if os.path.isdir(test):
    shutil.rmtree(test)

  print('Downloading %s %s ...' % (test, isolated_hash))
  cmd = [
    './isolated%s' % bin_suffix(),
    'download',
    '-I',
    isolateserver,
    '-isolated',
    isolated_hash,
    '-output-dir',
    test,
    '-cache-dir',
    'cache',
  ]
  print(cmd)
  return subprocess.call(cmd)


def create_isolate_file(test):
  files = []

  def walk(path):
    if os.path.islink(path):
      return
    if os.path.isfile(path):
      files.append(path)
      return
    for entry in os.listdir(path):
      walk(os.path.join(path, entry))

  walk(test)

  isolate = {
    'variables': {
      'files': files,
    }
  }
  filename = '%s.isolate' % test
  with open(filename, 'w') as f:
    json.dump(isolate, f)
  print('%s is created' % filename)


def create_isolated_gen_file(test):
  isolated_gen = {
    'args': [
      '--isolated',
      '%s.isolated' % test,
      '--isolate',
      '%s.isolate' % test,
    ],
    'dir': os.path.dirname(os.path.abspath(__file__)),
    'version': 1
  }

  filename = '%s.isolated.gen.json' % test
  with open(filename, 'w') as f:
    json.dump(isolated_gen, f)
  print('%s is created' % filename)


def upload_to_cas(cas_instance, isolated_tests):
  print('Uploading to CAS %s ...' % cas_instance)
  isolated_gen_files = ['%s.isolated.gen.json' % t for t in isolated_tests.keys()]
  cmd = [
    './isolate%s' % bin_suffix(),
    'batcharchive',
    '-verbose',
    '-cas-instance',
    cas_instance,
  ] + isolated_gen_files
  print(cmd)
  sys.stdout.flush()
  subprocess.call(cmd)
  sys.stdout.flush()


def main():
  parser = argparse.ArgumentParser(description='Download isolated tests from Isolate, and upload to CAS.')
  parser.add_argument('--isolateserver', type=str, default='isolateserver.appspot.com')
  parser.add_argument('--isolated_tests', type=str, default='isolated_tests.json')
  parser.add_argument('--cas_instance', type=str, default='chromium-swarm-dev')
  args = parser.parse_args()

  with open(args.isolated_tests) as f:
    isolated_tests = json.load(f)
  download_isolated_tests(args.isolateserver, isolated_tests)

  # 1st time, most files needs to be uploaded to CAS.
  print('Uploading to CAS: 1st')
  upload_to_cas(args.cas_instance, isolated_tests)

  # 2nd time, all files should be on CAS, so it's just file existence checks.
  print('Uploading to CAS: 2nd')
  upload_to_cas(args.cas_instance, isolated_tests)

if __name__ in '__main__':
  main()
