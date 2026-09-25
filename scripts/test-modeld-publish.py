import copy
import hashlib
import importlib.util
import io
import json
import pathlib
import tarfile
import tempfile
import unittest


spec = importlib.util.spec_from_file_location('publisher', pathlib.Path(__file__).with_name('modeld-publish.py'))
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)


class _Store:
    def __init__(self):
        self.objects = {}
        self.index = {'schema': 1, 'builds': [{'version': 'v1.0.0', 'platform': 'linux-amd64'}]}
        self.token = 0
        self.conflict = False
        self.always_conflict = False

    def immutable(self, key, path):
        content = path.read_bytes()
        if key in self.objects and self.objects[key] != content:
            raise ValueError('immutable object collision')
        self.objects[key] = content

    def read(self, key):
        return json.dumps(self.index).encode(), self.token

    def compare_and_swap(self, key, data, token):
        if self.always_conflict:
            return False
        if self.conflict:
            self.index['builds'].append({'version': 'v1.0.0', 'platform': 'windows-amd64'})
            self.token += 1
            self.conflict = False
        if self.token != token:
            return False
        self.index = json.loads(data)
        self.token += 1
        return True


class PublishTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.archive = pathlib.Path(self.temporary.name) / 'modeld-v2.0.0-darwin-arm64.tar.gz'
        manifest = {'modeld_version': 'v2.0.0', 'platform': 'darwin-arm64', 'protocol': 1, 'backends': ['llama']}
        content = json.dumps(manifest).encode()
        with tarfile.open(self.archive, 'w:gz') as bundle:
            info = tarfile.TarInfo('modeld-v2.0.0-darwin-arm64/manifest.json')
            info.size = len(content)
            bundle.addfile(info, io.BytesIO(content))
        self.metadata = {'version': 'v2.0.0', 'platform': 'darwin-arm64', 'protocol': 1,
                         'backends': ['llama'], 'channel': 'stable',
                         'archive': 'v2.0.0/' + self.archive.name,
                         'sha256': 'v2.0.0/' + self.archive.name + '.sha256',
                         'size': self.archive.stat().st_size}
        pathlib.Path(str(self.archive) + '.build.json').write_text(json.dumps(self.metadata))
        self.checksum = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        pathlib.Path(str(self.archive) + '.sha256').write_text(f'{self.checksum}  {self.archive.name}\n')

    def test_validates_real_archive_manifest_and_checksum(self):
        self.assertEqual(publisher._validate(self.archive), (self.metadata, self.checksum))
        self.archive.write_bytes(b'corrupted')
        with self.assertRaisesRegex(ValueError, 'checksum'):
            publisher._validate(self.archive)

    def test_wrong_manifest_rejected(self):
        self.metadata['protocol'] = 2
        pathlib.Path(str(self.archive) + '.build.json').write_text(json.dumps(self.metadata))
        with self.assertRaisesRegex(ValueError, 'manifest'):
            publisher._validate(self.archive)

    def test_adds_late_platform_without_losing_concurrent_platform(self):
        store = _Store()
        store.conflict = True
        publisher._publish(store, self.archive, self.metadata, lambda: None)
        self.assertEqual({entry['platform'] for entry in store.index['builds']},
                         {'linux-amd64', 'windows-amd64', 'darwin-arm64'})
        self.assertTrue(any(key.startswith('index-history/') for key in store.objects))
        token = store.token
        publisher._publish(store, self.archive, self.metadata, lambda: None)
        self.assertEqual(store.token, token)

    def test_collision_does_not_change_index(self):
        store = _Store()
        before = copy.deepcopy(store.index)
        store.objects[self.metadata['archive']] = b'other release'
        with self.assertRaisesRegex(ValueError, 'collision'):
            publisher._publish(store, self.archive, self.metadata, lambda: None)
        self.assertEqual(store.index, before)

    def test_unreadable_public_object_does_not_change_index(self):
        store = _Store()
        before = copy.deepcopy(store.index)
        def unreadable():
            raise RuntimeError('public read denied')
        with self.assertRaisesRegex(RuntimeError, 'public read'):
            publisher._publish(store, self.archive, self.metadata, unreadable)
        self.assertEqual(store.index, before)

    def test_repeated_race_fails_without_clobber(self):
        store = _Store()
        before = copy.deepcopy(store.index)
        store.always_conflict = True
        with self.assertRaisesRegex(RuntimeError, 'index kept changing'):
            publisher._publish(store, self.archive, self.metadata, lambda: None)
        self.assertEqual(store.index, before)


if __name__ == '__main__':
    unittest.main()
