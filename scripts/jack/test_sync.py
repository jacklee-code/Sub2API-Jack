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

class SyncTest(unittest.TestCase):
    def fixture(self, tmp, conflict=False):
        root = Path(tmp)
        origin = root / 'origin.git'; upstream = root / 'upstream.git'; work = root / 'work'; author = root / 'author'
        def git(cwd, *args):
            return subprocess.check_output(['git', *args], cwd=cwd, text=True, stderr=subprocess.DEVNULL).strip()
        git(root, 'init', '--bare', str(origin)); git(root, 'init', '--bare', str(upstream))
        git(root, 'init', '-b', 'main', str(author))
        git(author, 'config', 'user.name', 'Fixture'); git(author, 'config', 'user.email', 'fixture@example.test')
        (author/'base.txt').write_text('original\n'); git(author, 'add', '.'); git(author, 'commit', '-m', 'base')
        base = git(author, 'rev-parse', 'HEAD'); git(author, 'remote', 'add', 'upstream', str(upstream)); git(author, 'push', 'upstream', 'main')
        git(root, 'clone', '-b', 'main', str(upstream), str(work)); git(work, 'remote', 'set-url', 'origin', str(origin))
        git(work, 'config', 'user.name', 'Fixture'); git(work, 'config', 'user.email', 'fixture@example.test')
        (work/'.jack').mkdir(); (work/'.jack/upstream.json').write_text(json.dumps({'tag':'v0.2.13','commit':base}))
        (work/'custom.txt').write_text('keep custom fleet feature\n')
        if conflict: (work/'base.txt').write_text('downstream edits\n')
        git(work, 'add', '.'); git(work, 'commit', '-m', 'downstream feature'); git(work, 'push', '-u', 'origin', 'main')
        (author/('base.txt' if conflict else 'official.txt')).write_text('official release update\n')
        git(author, 'add', '.'); git(author, 'commit', '-m', 'official update'); git(author, 'tag', 'v0.2.14'); git(author, 'push', 'upstream', 'main', '--tags')
        target = git(author, 'rev-parse', 'HEAD')
        return work, upstream, target, git

    def test_real_merge_preserves_custom_feature(self):
        self.run_case(False)

    def test_conflict_opens_draft_without_replacing_custom_code(self):
        self.run_case(True)

    def run_case(self, conflict):
        with tempfile.TemporaryDirectory() as tmp:
            work, upstream, target, git = self.fixture(tmp, conflict)
            commands = []
            def command(*args, check=True):
                commands.append(args)
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
            if conflict:
                self.assertIn('--draft', creates[0]); self.assertTrue((work/'.jack/upstream-conflict.md').exists())
                self.assertEqual((work/'base.txt').read_text(), 'downstream edits\n')
                self.assertIn('ready=false', (Path(tmp)/'output').read_text())
            else:
                self.assertEqual((work/'official.txt').read_text(), 'official release update\n')
                self.assertEqual(json.loads((work/'.jack/upstream.json').read_text())['commit'], target)
                git(work, 'merge-base', '--is-ancestor', target, 'HEAD')
                self.assertIn('ready=true', (Path(tmp)/'output').read_text())

if __name__ == '__main__': unittest.main()
