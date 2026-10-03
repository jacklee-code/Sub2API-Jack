import importlib.util
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

if __name__ == '__main__': unittest.main()
