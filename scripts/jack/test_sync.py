import base64
from contextlib import redirect_stderr
import importlib.util
import io
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

class CommandTest(unittest.TestCase):
    def test_real_failure_logs_both_streams_and_retains_captured_output(self):
        for check in (True, False):
            with self.subTest(check=check):
                log = io.StringIO()
                command = (sys.executable, '-c', 'import sys; print("fixture stdout"); print("fixture stderr", file=sys.stderr); sys.exit(7)')
                with redirect_stderr(log):
                    if check:
                        with self.assertRaises(subprocess.CalledProcessError) as raised:
                            sync.run(*command)
                        result = raised.exception
                    else:
                        result = sync.run(*command, check=False)
                self.assertEqual(result.returncode, 7)
                self.assertEqual(result.stdout.strip(), 'fixture stdout')
                self.assertEqual(result.stderr.strip(), 'fixture stderr')
                self.assertIn('fixture stdout', log.getvalue())
                self.assertIn('fixture stderr', log.getvalue())

    def test_failure_redacts_tokens_and_temporary_git_header(self):
        token = 'fixture-token-not-a-credential'
        encoded = base64.b64encode(f'x-access-token:{token}'.encode()).decode()
        env = {**os.environ, 'GH_TOKEN': 'fixture-default-token', 'GIT_CONFIG_VALUE_1': f'AUTHORIZATION: basic {encoded}'}
        log = io.StringIO()
        with redirect_stderr(log):
            result = sync.run(sys.executable, '-c', 'import base64,os,sys; print(os.environ["GH_TOKEN"]); print(base64.b64decode(os.environ["GIT_CONFIG_VALUE_1"].split()[-1]).decode()); print(os.environ["GIT_CONFIG_VALUE_1"], file=sys.stderr); sys.exit(1)', check=False, env=env)
        self.assertEqual(result.returncode, 1)
        self.assertIn(token, result.stdout)
        self.assertIn('***', log.getvalue())
        self.assertNotIn(token, log.getvalue())
        self.assertNotIn('fixture-default-token', log.getvalue())
        self.assertNotIn(encoded, log.getvalue())

    def test_workflow_push_rejection_explains_fix_and_still_fails(self):
        error = subprocess.CalledProcessError(1, ['git', 'push'], stderr='refusing to allow a GitHub App to create or update workflow .github/workflows/test.yml without workflows permission')
        log = io.StringIO()
        with patch.object(sync, 'run', side_effect=error), redirect_stderr(log):
            with self.assertRaises(subprocess.CalledProcessError) as raised:
                sync.push_branch('sync/upstream-v0.2.14', '')
        self.assertIs(raised.exception, error)
        self.assertIn('JACK_SYNC_PUSH_TOKEN', log.getvalue())
        self.assertIn('Contents and Workflows write', log.getvalue())

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

    def sync(self, tmp, edits, *, existing=None, wire_fails=False, push_token=''):
        work, upstream, target, git = self.fixture(tmp, edits)
        if existing == 'repaired':
            git(work, 'fetch', str(upstream), 'main')
            git(work, 'merge', '--no-ff', '--no-edit', target)
            (work/'.jack/upstream.json').write_text(json.dumps({'tag': 'v0.2.14', 'commit': target}))
            git(work, 'add', '.'); git(work, 'commit', '-m', 'repair')
            sha = git(work, 'rev-parse', 'HEAD')
            git(work, 'push', 'origin', 'HEAD:refs/heads/sync/upstream-v0.2.14')
            git(work, 'checkout', '--detach', 'origin/main')
            existing = [{'number': 28, 'isDraft': False, 'headRefOid': sha,
                         'baseRefName': 'main', 'isCrossRepository': False}]
        commands = []
        push_environments = []
        def command(*args, check=True, env=None):
            commands.append(args)
            self.assertNotIn('JACK_SYNC_PUSH_TOKEN', os.environ)
            if env is not None:
                self.assertEqual(args[:3], ('git', 'push', 'origin'))
                push_environments.append(env)
            if args[0] == 'go':
                self.assertEqual((work/'backend/cmd/server/wire_gen.go').read_text(), 'jack wire\n')
                self.assertEqual((work/'backend/cmd/server/wire.go').read_text(), 'merged providers\n')
                if wire_fails:
                    return subprocess.CompletedProcess(args, 1, '', 'missing provider')
                (work/'backend/cmd/server/wire_gen.go').write_text('generated with Jack and upstream providers\n')
                return subprocess.CompletedProcess(args, 0, '', '')
            if args[0] == 'pnpm':
                self.assertEqual((work/'frontend/pnpm-lock.yaml').read_text(), 'jack lock\n')
                (work/'frontend/pnpm-lock.yaml').write_text('regenerated\n')
                return subprocess.CompletedProcess(args, 0, '', '')
            if args[0] == 'gh':
                if args[1] == 'api': out = json.dumps({'tag_name':'v0.2.14','draft':False,'prerelease':False,'html_url':'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.14'})
                elif args[1:3] == ('pr','list'): out = json.dumps(existing or [])
                else:
                    self.assertIn('Integrate official', Path(args[args.index('--body-file') + 1]).read_text())
                    out = 'https://github.com/jacklee-code/Sub2API-Jack/pull/123'
                return subprocess.CompletedProcess(args, 0, out, '')
            args = list(args)
            if args[:3] == ['git','fetch','https://github.com/Wei-Shaw/sub2api.git']: args[2] = str(upstream)
            return subprocess.run(args, cwd=work, text=True, capture_output=True, check=check, env=env)
        with patch.object(sync, 'ROOT', work), patch.object(sync, 'run', command), patch.object(sys, 'argv', ['sync-upstream.py']), patch.dict(os.environ, {'GITHUB_OUTPUT':str(Path(tmp)/'output'), 'JACK_SYNC_PUSH_TOKEN': push_token}):
            sync.main()
        self.assertEqual(len(push_environments), int(bool(push_token) and not existing))
        if push_environments:
            env = push_environments[0]
            self.assertNotIn('JACK_SYNC_PUSH_TOKEN', env)
            last = int(env['GIT_CONFIG_COUNT']) - 1
            self.assertEqual(env[f'GIT_CONFIG_VALUE_{last - 1}'], '')
            self.assertEqual(env[f'GIT_CONFIG_VALUE_{last}'], 'AUTHORIZATION: basic ' + base64.b64encode(f'x-access-token:{push_token}'.encode()).decode())
            self.assertNotIn(push_token, git(work, 'config', '--local', '--list'))
        self.assertEqual((work/'custom.txt').read_text(), 'keep custom fleet feature\n')
        creates = [c for c in commands if c[:3] == ('gh','pr','create')]
        if existing:
            self.assertFalse(creates)
            self.assertFalse([c for c in commands if c[:2] in [('git', 'switch'), ('git', 'push')]])
            return (Path(tmp)/'output').read_text()
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

    def test_dependency_version_conflicts_keep_newer_versions_on_both_sides(self):
        with tempfile.TemporaryDirectory() as tmp:
            prefix = 'module example.test/app\n\ngo 1.27.3\n\nrequire (\n'
            suffix = ')\n'
            work, target, git, create, _, output = self.sync(tmp, {
                'backend/go.mod': tuple(prefix + value + suffix for value in (
                    '\tgolang.org/x/crypto v0.56.0\n\tgolang.org/x/image v0.41.0\n',
                    '\tgolang.org/x/crypto v0.57.0\n\tgolang.org/x/image v0.45.0\n',
                    '\tgolang.org/x/crypto v0.58.0\n\tgolang.org/x/image v0.41.0\n'))})
            self.assertReady(work, target, git, create, output)
            self.assertIn('crypto v0.58.0', (work/'backend/go.mod').read_text())
            self.assertIn('image v0.45.0', (work/'backend/go.mod').read_text())

    def test_dependency_directive_conflict_stays_blocked(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, _, _, create, _, output = self.sync(tmp, {
                'backend/go.mod': ('go 1.27.1\n', 'go 1.27.3\n', 'go 1.28.0\n')})
            self.assertIn('--draft', create)
            self.assertIn('blocked=true', output)

    def test_locale_entry_additions_preserve_jack_and_upstream(self):
        base = "import common from './common'\n\nexport default {\n  ...common,\n}\n"
        jack = "import fleet from './fleet'\nimport chat from './chat'\n" + base.replace('export default {', 'export default {\n  ...fleet,\n  ...chat,')
        official = "import adminAccessKeys from './adminAccessKeys'\n" + base.replace('export default {', 'export default {\n  adminAccessKeys,')
        with tempfile.TemporaryDirectory() as tmp:
            work, target, git, create, _, output = self.sync(tmp, {
                'frontend/src/i18n/locales/en/index.ts': (base, jack, official)})
            self.assertReady(work, target, git, create, output)
            merged = (work/'frontend/src/i18n/locales/en/index.ts').read_text()
            for value in ['...fleet,', '...chat,', '  adminAccessKeys,', '...common,']:
                self.assertIn(value, merged)

    def test_locale_semantic_edits_stay_blocked(self):
        with tempfile.TemporaryDirectory() as tmp:
            _, _, _, create, _, output = self.sync(tmp, {
                'frontend/src/i18n/locales/en/index.ts': ('original\n', 'custom changes\n', 'upstream changes\n')})
            self.assertIn('--draft', create)
            self.assertIn('blocked=true', output)

    def test_optional_token_is_only_given_to_push_not_regeneration_or_gh(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, target, git, create, _, output = self.sync(tmp, {
                'frontend/pnpm-lock.yaml': ('lock\n', 'jack lock\n', 'official lock\n')},
                push_token='fixture-push-token-not-a-credential')
            self.assertReady(work, target, git, create, output)

    def test_wire_regenerated_from_merged_providers(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, target, git, create, _, output = self.sync(tmp, {
                'backend/cmd/server/wire_gen.go': ('base wire\n', 'jack wire\n', 'upstream wire\n'),
                'backend/cmd/server/wire.go': ('old providers\n', None, 'merged providers\n')})
            self.assertReady(work, target, git, create, output)
            self.assertEqual((work/'backend/cmd/server/wire_gen.go').read_text(), 'generated with Jack and upstream providers\n')
            self.assertEqual(git(work, 'status', '--porcelain'), '')

    def test_failed_wire_generation_preserves_draft_and_original_code(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, _, _, create, _, output = self.sync(tmp, {
                'backend/cmd/server/wire_gen.go': ('base wire\n', 'jack wire\n', 'upstream wire\n'),
                'backend/cmd/server/wire.go': ('old providers\n', None, 'merged providers\n')}, wire_fails=True)
            self.assertIn('--draft', create)
            self.assertIn('blocked=true', output)
            self.assertEqual((work/'backend/cmd/server/wire_gen.go').read_text(), 'jack wire\n')

    def test_repaired_existing_pr_resumes_only_with_verified_ancestry_and_metadata(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, upstream, target, git = self.fixture(tmp, {'official.txt': ('old\n', None, 'new\n')})
            git(work, 'fetch', str(upstream), 'main')
            git(work, 'merge', '--no-ff', '--no-edit', target)
            (work/'.jack/upstream.json').write_text(json.dumps({'tag': 'v0.2.14', 'commit': target}))
            git(work, 'add', '.jack/upstream.json'); git(work, 'commit', '-m', 'record repair')
            branch = 'sync/upstream-v0.2.14'
            git(work, 'push', 'origin', 'HEAD:refs/heads/' + branch)
            pr = {'number': 28, 'isDraft': False, 'headRefOid': git(work, 'rev-parse', 'HEAD'),
                  'baseRefName': 'main', 'isCrossRepository': False}
            with patch.object(sync, 'ROOT', work):
                self.assertIsNone(sync.existing_pr_blocker(pr, branch, 'v0.2.14', target))
                for changes in ({'isDraft': True}, {'baseRefName': 'other'}, {'isCrossRepository': True}, {'headRefOid': '0' * 40}):
                    self.assertIsNotNone(sync.existing_pr_blocker({**pr, **changes}, branch, 'v0.2.14', target))
                self.assertIsNotNone(sync.existing_pr_blocker(pr, branch, 'v0.2.15', target))
                (work/'.jack/upstream-conflict.md').write_text('still blocked')
                git(work, 'add', '.'); git(work, 'commit', '-m', 'still blocked')
                git(work, 'push', 'origin', 'HEAD:refs/heads/' + branch)
                pr['headRefOid'] = git(work, 'rev-parse', 'HEAD')
                self.assertIsNotNone(sync.existing_pr_blocker(pr, branch, 'v0.2.14', target))
            self.assertEqual(git(work, 'status', '--porcelain'), '')

    def test_main_resumes_repaired_pr_without_push_or_recreating_it(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = self.sync(tmp, {'official.txt': ('old\n', None, 'new\n')}, existing='repaired')
            self.assertIn('ready=true', output)
            self.assertIn('blocked=false', output)
            self.assertIn('reason=resume_existing_pr', output)
            self.assertIn('pr=https://github.com/jacklee-code/Sub2API-Jack/pull/28', output)
            self.assertRegex(output, r'sha=[0-9a-f]{40}')

    def test_unrepaired_existing_pr_reports_blocked_without_overwriting_repairs(self):
        for draft in (True, False):
            with self.subTest(draft=draft), tempfile.TemporaryDirectory() as tmp:
                output = self.sync(tmp, {'official.txt': ('old\n', None, 'new\n')},
                                   existing=[{'number': 28, 'isDraft': draft, 'headRefOid': 'repair'}])
                self.assertIn('blocked=true', output)
                self.assertIn('ready=false', output)
                self.assertIn('reason=existing_pr', output)

if __name__ == '__main__': unittest.main()
