import toml
import os
import sys
import zipfile
import tarfile
import contextlib
import tempfile
from glob import glob


def parse_manifest(manifest_path):
    with open(manifest_path, 'rt') as f:
        return toml.load(f)


def tg_home():
    return os.environ.get('TESTGROUND_HOME',
                          os.path.join(os.environ['HOME'], 'testground'))


def get_plans():
    return list(os.listdir(os.path.join(tg_home(), 'plans')))


def get_manifest(plan_name):
    manifest_path = os.path.join(tg_home(), 'plans', plan_name, 'manifest.toml')
    return parse_manifest(manifest_path)


def print_err(*args):
    print(*args, file=sys.stderr)


# sugar around recursive glob search
def find_files(dirname, filename_glob):
    path = '{}/**/{}'.format(dirname, filename_glob)
    return glob(path, recursive=True)


# depending on which test runner was used, the collection archive may be either a zip (local docker & exec runner),
# or a tar.gz file (k8s). Unfortunately, the zipfile and tarfile modules are different species of waterfowl,
# so we duck typing doesn't help. So this method extracts whichever one we have to a temp directory and
# returns the path to the temp dir.
# use as a context manager, so the temp dir gets deleted when we're done:
#   with open_archive(archive_path) as a:
#     files = glob(a + '/**/tracer-output')
@contextlib.contextmanager
def open_archive(archive_path):
    with archive_temp_dir() as d:
        extract_archive(archive_path, d)
        yield d


def archive_temp_dir():
    return tempfile.TemporaryDirectory(prefix='oni-test-archive-')


def extract_archive(archive_path, dest):
    if zipfile.is_zipfile(archive_path):
        z = zipfile.ZipFile(archive_path)
    else:
        z = tarfile.open(archive_path, 'r:gz')
    z.extractall(path=dest)
