import argparse
import hashlib
import json
import pathlib
import re
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile


def _digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def _validate(archive):
    metadata = json.loads(pathlib.Path(str(archive) + '.build.json').read_text())
    version, platform = metadata['version'], metadata['platform']
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?', version):
        raise ValueError('invalid worker version')
    if not re.fullmatch(r'(linux|darwin|windows)-(amd64|arm64)', platform):
        raise ValueError('invalid worker platform')
    extension = '.zip' if platform.startswith('windows-') else '.tar.gz'
    name = f'modeld-{version}-{platform}'
    if archive.name != name + extension:
        raise ValueError('archive filename disagrees with metadata')
    if metadata['archive'] != f'{version}/{archive.name}' or metadata['sha256'] != f'{version}/{archive.name}.sha256':
        raise ValueError('metadata paths do not match the release layout')
    if metadata['channel'] != 'stable' or type(metadata['protocol']) is not int or metadata['protocol'] < 1:
        raise ValueError('invalid channel or protocol')
    if not metadata['backends'] or not set(metadata['backends']) <= {'llama', 'openvino'}:
        raise ValueError('invalid backend set')
    checksum = pathlib.Path(str(archive) + '.sha256')
    fields = checksum.read_text().strip().split()
    if len(fields) != 2 or fields[1].lstrip('*') != archive.name or fields[0] != _digest(archive):
        raise ValueError('archive checksum mismatch')
    if metadata['size'] != archive.stat().st_size:
        raise ValueError('archive size mismatch')
    if extension == '.zip':
        with zipfile.ZipFile(archive) as bundle:
            manifest = json.loads(bundle.read(name + '/manifest.json'))
    else:
        with tarfile.open(archive) as bundle:
            manifest = json.load(bundle.extractfile(name + '/manifest.json'))
    for field, manifest_field in [('version', 'modeld_version'), ('platform', 'platform'),
                                  ('protocol', 'protocol'), ('backends', 'backends')]:
        if metadata[field] != manifest[manifest_field]:
            raise ValueError(f'manifest disagrees with metadata: {field}')
    return metadata, fields[0]


class _S3:
    def __init__(self, uri):
        match = re.fullmatch(r's3://([^/]+)/(.+)', uri.rstrip('/'))
        if not match:
            raise ValueError('release store must be s3://bucket/prefix')
        self.bucket, self.prefix = match.groups()

    def _command(self, operation, key, *args):
        return subprocess.run(['aws', 's3api', operation, '--bucket', self.bucket,
                               '--key', self.prefix + '/' + key, *args,
                               '--output', 'json', '--no-cli-pager'], capture_output=True, text=True)

    def read(self, key):
        with tempfile.TemporaryDirectory() as temporary:
            path = pathlib.Path(temporary) / 'object'
            result = self._command('get-object', key, str(path))
            if result.returncode:
                if '(NoSuchKey)' in result.stderr or '(404)' in result.stderr:
                    return None, None
                raise RuntimeError(result.stderr)
            return path.read_bytes(), json.loads(result.stdout)['ETag']

    def immutable(self, key, path):
        result = self._command('put-object', key, '--body', str(path), '--if-none-match', '*')
        if result.returncode:
            if '(PreconditionFailed)' not in result.stderr and '(ConditionalRequestConflict)' not in result.stderr:
                raise RuntimeError(result.stderr)
            with tempfile.TemporaryDirectory() as temporary:
                existing = pathlib.Path(temporary) / 'object'
                fetched = self._command('get-object', key, str(existing))
                if fetched.returncode:
                    raise RuntimeError(fetched.stderr)
                if _digest(existing) != _digest(path):
                    raise ValueError(f'refusing to replace published object: {key}')

    def compare_and_swap(self, key, data, token):
        with tempfile.TemporaryDirectory() as temporary:
            path = pathlib.Path(temporary) / 'index.json'
            path.write_bytes(data)
            condition = ['--if-none-match', '*'] if token is None else ['--if-match', token]
            result = self._command('put-object', key, '--body', str(path), '--content-type',
                                   'application/json', '--cache-control', 'no-cache', *condition)
            if result.returncode:
                if '(PreconditionFailed)' in result.stderr or '(ConditionalRequestConflict)' in result.stderr:
                    return False
                raise RuntimeError(result.stderr)
            return True


def _public_check(base_url, metadata, checksum):
    if not base_url.startswith('https://'):
        raise ValueError('public base URL must use HTTPS')
    request = urllib.request.Request(base_url.rstrip('/') + '/' + metadata['archive'], method='HEAD')
    with urllib.request.urlopen(request, timeout=30) as response:
        if int(response.headers['Content-Length']) != metadata['size']:
            raise ValueError('public archive size differs from local artifact')
    with urllib.request.urlopen(base_url.rstrip('/') + '/' + metadata['sha256'], timeout=30) as response:
        if response.read(4096).decode().split()[0] != checksum:
            raise ValueError('public checksum differs from local artifact')


def _publish(store, archive, metadata, public_check):
    store.immutable(metadata['archive'], archive)
    store.immutable(metadata['sha256'], pathlib.Path(str(archive) + '.sha256'))
    store.immutable(metadata['archive'] + '.build.json', pathlib.Path(str(archive) + '.build.json'))
    public_check()
    for attempt in range(8):
        data, token = store.read('index.json')
        index = {'schema': 1, 'builds': []} if data is None else json.loads(data)
        if index.get('schema') != 1 or not isinstance(index.get('builds'), list):
            raise ValueError('unsupported existing release index')
        matches = [entry for entry in index['builds'] if
                   (entry['version'], entry['platform']) == (metadata['version'], metadata['platform'])]
        if matches:
            if len(matches) == 1 and matches[0] == metadata:
                return
            raise ValueError('version/platform already indexed with different metadata')
        index['builds'].append(metadata)
        index['builds'].sort(key=lambda entry: (entry['version'], entry['platform']))
        if data is not None:
            with tempfile.TemporaryDirectory() as temporary:
                previous = pathlib.Path(temporary) / 'previous.json'
                previous.write_bytes(data)
                store.immutable('index-history/' + hashlib.sha256(data).hexdigest() + '.json', previous)
        if store.compare_and_swap('index.json', (json.dumps(index, indent=2) + '\n').encode(), token):
            return
    raise RuntimeError('index kept changing; immutable artifacts uploaded, index not updated; retry publication')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Publish an immutable worker and merge it into the installer index.')
    parser.add_argument('archive', type=pathlib.Path)
    parser.add_argument('--store')
    parser.add_argument('--public-base-url')
    parser.add_argument('--check-only', action='store_true')
    parser.add_argument('--linux-only', action='store_true')
    args = parser.parse_args()
    metadata, checksum = _validate(args.archive)
    if args.linux_only and not metadata['platform'].startswith('linux-'):
        parser.error('this release path supports Linux workers only')
    if args.check_only:
        print(json.dumps(metadata, indent=2))
    else:
        if not args.store or not args.public_base_url:
            parser.error('--store and --public-base-url are required for publication')
        if not args.public_base_url.startswith('https://'):
            parser.error('--public-base-url must use HTTPS')
        _publish(_S3(args.store), args.archive, metadata,
                 lambda: _public_check(args.public_base_url, metadata, checksum))
        print(f'Published {metadata["platform"]} {metadata["version"]}: {args.public_base_url.rstrip("/")}/index.json')
