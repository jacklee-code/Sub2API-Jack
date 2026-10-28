import importlib.util
import hashlib
import tarfile
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('jack_release', Path(__file__).with_name('release.py'))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)

class ReleaseTest(unittest.TestCase):
    def test_version_format(self):
        for value in ['0.2.13-jack.1', '1.0.0-jack.22']:
            self.assertIsNotNone(release.VERSION.fullmatch(value))
        for value in ['0.2.13', '0.2.13-jack.0', '0.2.13-jack.1;echo secret', '../bad']:
            self.assertIsNone(release.VERSION.fullmatch(value))

    def test_runtime_changes_with_dependencies_not_features(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in ['Dockerfile.goreleaser','deploy/docker-entrypoint.sh','deploy/jack/entrypoint.sh','backend/resources/catalog.json']:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('original')
            with patch.object(release, 'ROOT', root):
                original = release.runtime_id()
                (root / 'feature.go').write_text('new feature')
                self.assertEqual(original, release.runtime_id())
                (root / 'backend/resources/catalog.json').write_text('new runtime resource')
                self.assertNotEqual(original, release.runtime_id())

    def test_bundled_pricing_does_not_change_runtime_but_image_inputs_do(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            inputs = ['Dockerfile.goreleaser', 'deploy/docker-entrypoint.sh', 'deploy/jack/entrypoint.sh']
            pricing = 'backend/resources/model-pricing/model_prices_and_context_window.json'
            for name in inputs + [pricing]:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('original')
            with patch.object(release, 'ROOT', root):
                original = release.runtime_id()
                (root / pricing).write_text('new pricing bundled in the binary')
                self.assertEqual(original, release.runtime_id())
                for name in inputs:
                    with self.subTest(path=name):
                        (root / name).write_text('changed image requirement')
                        self.assertNotEqual(original, release.runtime_id())
                        (root / name).write_text('original')
                (root / 'backend/resources/new-runtime-data.json').write_text('not embedded')
                self.assertNotEqual(original, release.runtime_id())

    def test_package_marks_vendor_pricing_without_separating_it_from_binary_updates(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            files = {
                'Dockerfile.goreleaser': 'FROM alpine:3.21\n',
                'deploy/docker-entrypoint.sh': '#!/bin/sh\n',
                'deploy/jack/entrypoint.sh': '#!/bin/sh\n',
                'backend/resources/model-pricing/model_prices_and_context_window.json': '{"fixture": {}}',
                'LICENSE': 'fixture license',
                'sub2api': 'fixture executable with embedded resources',
            }
            for name, content in files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content)
            with patch.object(release, 'ROOT', root), patch.object(release, 'metadata', return_value={'version': '0.2.19-jack.2'}):
                release.package('0.2.19-jack.2', 'arm64', root / 'sub2api')
            pricing = root / '.jack-build/arm64/backend/resources/model-pricing/model_prices_and_context_window.json'
            self.assertEqual(pricing.with_suffix('.json.jack-sha256').read_text().strip(), hashlib.sha256(pricing.read_bytes()).hexdigest())
            with tarfile.open(root / '.jack-dist/sub2api_0.2.19-jack.2_linux_arm64.tar.gz') as archive:
                self.assertEqual(archive.getnames(), ['sub2api', 'LICENSE'])
                self.assertEqual(archive.extractfile('sub2api').read(), (root / 'sub2api').read_bytes())

if __name__ == '__main__': unittest.main()
