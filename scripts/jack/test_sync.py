import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('jack_sync', Path(__file__).with_name('sync-upstream.py'))
sync = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync)

POLICY = Path(__file__).resolve().parents[2] / '.jack/merge-policy.json'

class SyncTest(unittest.TestCase):
    def fixture(self, tmp, edits):
        """edits: path -> (original, Jack, official); a Jack value of None leaves the file untouched."""
        root = Path(tmp)
        origin = root / 'origin.git'; upstream = root / 'upstream.git'; work = root / 'work'; author = root / 'author'
        def git(cwd, *args):
            return subprocess.check_output(['git', *args], cwd=cwd, text=True, stderr=subprocess.DEVNULL).strip()
        def write(base, path, text):
            (base/path).parent.mkdir(parents=True, exist_ok=True); (base/path).write_text(text)
        git(root, 'init', '--bare', str(origin)); git(root, 'init', '--bare', str(upstream))
        git(root, 'init', '-b', 'main', str(author))
        git(author, 'config', 'user.name', 'Fixture'); git(author, 'config', 'user.email', 'fixture@example.test')
        for path, (original, _, _) in edits.items(): write(author, path, original)
        git(author, 'add', '.'); git(author, 'commit', '-m', 'base')
        base = git(author, 'rev-parse', 'HEAD'); git(author, 'remote', 'add', 'upstream', str(upstream)); git(author, 'push', 'upstream', 'main')
        git(root, 'clone', '-b', 'main', str(upstream), str(work)); git(work, 'remote', 'set-url', 'origin', str(origin))
        git(work, 'config', 'user.name', 'Fixture'); git(work, 'config', 'user.email', 'fixture@example.test')
        write(work, '.jack/upstream.json', json.dumps({'tag':'v0.2.13','commit':base}))
        write(work, '.jack/merge-policy.json', POLICY.read_text())
        write(work, 'custom.txt', 'keep custom fleet feature\n')
        for path, (_, jack, _) in edits.items():
            if jack is not None: write(work, path, jack)
        git(work, 'add', '.'); git(work, 'commit', '-m', 'downstream feature'); git(work, 'push', '-u', 'origin', 'main')
        for path, (_, _, official) in edits.items(): write(author, path, official)
        git(author, 'add', '.'); git(author, 'commit', '-m', 'official update'); git(author, 'tag', 'v0.2.14'); git(author, 'push', 'upstream', 'main', '--tags')
        target = git(author, 'rev-parse', 'HEAD')
        return work, upstream, target, git

    def sync(self, tmp, edits):
        work, upstream, target, git = self.fixture(tmp, edits)
        commands = []
        def command(*args, check=True):
            commands.append(args)
            if args[0] == 'pnpm':
                self.assertEqual((work/'frontend/pnpm-lock.yaml').read_text(), 'jack lock\n')
                (work/'frontend/pnpm-lock.yaml').write_text('regenerated\n')
                return subprocess.CompletedProcess(args, 0, '', '')
            if args[0] == 'gh':
                if args[1] == 'api': out = json.dumps({'tag_name':'v0.2.14','draft':False,'prerelease':False,'html_url':'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.14'})
                elif args[1:3] == ('pr','list'): out = '[]'
                else: out = 'https://github.com/jacklee-code/Sub2API-Jack/pull/123'
                return subprocess.CompletedProcess(args, 0, out, '')
            args = list(args)
            if args[:3] == ['git','fetch','https://github.com/Wei-Shaw/sub2api.git']: args[2] = str(upstream)
            return subprocess.run(args, cwd=work, text=True, capture_output=True, check=check)
        with patch.object(sync, 'ROOT', work), patch.object(sync, 'run', command), patch.object(sys, 'argv', ['sync-upstream.py']), patch.dict(os.environ, {'GITHUB_OUTPUT':str(Path(tmp)/'output')}):
            sync.main()
        self.assertEqual((work/'custom.txt').read_text(), 'keep custom fleet feature\n')
        creates = [c for c in commands if c[:3] == ('gh','pr','create')]
        self.assertEqual(len(creates), 1)
        self.assertIn('jacklee-code', creates[0])
        return work, target, git, creates[0], commands, (Path(tmp)/'output').read_text()

    def assertReady(self, work, target, git, create, output):
        self.assertNotIn('--draft', create); self.assertFalse((work/'.jack/upstream-conflict.md').exists())
        git(work, 'merge-base', '--is-ancestor', target, 'HEAD')
        self.assertEqual(json.loads((work/'.jack/upstream.json').read_text())['commit'], target)
        self.assertIn('ready=true', output)

    def test_real_merge_preserves_custom_feature(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, target, git, create, _, output = self.sync(tmp, {'official.txt': ('', None, 'official release update\n')})
            self.assertReady(work, target, git, create, output)
            self.assertEqual((work/'official.txt').read_text(), 'official release update\n')

    def test_conflict_opens_draft_without_replacing_custom_code(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, target, git, create, commands, output = self.sync(tmp, {
                'base.txt': ('original\n', 'downstream edits\n', 'official release update\n'),
                'frontend/pnpm-lock.yaml': ('lock\n', 'jack lock\n', 'official lock\n')})
            self.assertIn('--draft', create); self.assertTrue((work/'.jack/upstream-conflict.md').exists())
            self.assertEqual((work/'base.txt').read_text(), 'downstream edits\n')
            self.assertFalse([c for c in commands if c[0] == 'pnpm'])
            self.assertIn('ready=false', output); self.assertIn('blocked=true', output)

    def test_policy_resolves_lockfile_upstream_and_union_conflicts(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, target, git, create, _, output = self.sync(tmp, {
                'frontend/pnpm-lock.yaml': ('lock\n', 'jack lock\n', 'official lock\n'),
                '.github/audit-exceptions.yml': ('expires: 1\n', 'expires: 3\n', 'expires: 2\n'),
                'backend/go.sum': ('a v1\n', 'a v1\nb v1\n', 'a v1\nc v1\n')})
            self.assertReady(work, target, git, create, output)
            self.assertEqual((work/'frontend/pnpm-lock.yaml').read_text(), 'regenerated\n')
            self.assertEqual((work/'.github/audit-exceptions.yml').read_text(), 'expires: 2\n')
            self.assertEqual((work/'backend/go.sum').read_text(), 'a v1\nb v1\nc v1\n')

if __name__ == '__main__': unittest.main()
