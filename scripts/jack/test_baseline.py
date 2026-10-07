import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('test-baseline.py')
spec = importlib.util.spec_from_file_location('jack_baseline', SCRIPT)
baseline = importlib.util.module_from_spec(spec)
spec.loader.exec_module(baseline)

def go_events(*events):
    return ''.join(json.dumps(event) + '\n' for event in events)

class BaselineTest(unittest.TestCase):
    def test_go_failures_report_tests_and_package_only_failures(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'go.json'
            path.write_text('not json\n' + go_events(
                {'Action': 'output', 'Package': 'a', 'Test': 'TestX', 'Output': 'boom\n'},
                {'Action': 'fail', 'Package': 'a', 'Test': 'TestX'},
                {'Action': 'fail', 'Package': 'a'},
                {'Action': 'fail', 'Package': 'b'},
                {'Action': 'pass', 'Package': 'c', 'Test': 'TestY'}))
            failed, output = baseline.go_failures([path])
            self.assertEqual(failed, {'a TestX', 'b [package]'})
            self.assertEqual(output['a TestX'], ['boom\n'])

    def test_vitest_failures_are_relative_to_root(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / 'frontend'
            report = {'testResults': [
                {'name': str(root / 'src/a.spec.ts'), 'status': 'failed', 'assertionResults': [
                    {'fullName': 'A > breaks', 'status': 'failed', 'failureMessages': ['x']},
                    {'fullName': 'A > works', 'status': 'passed'}]},
                {'name': str(root / 'src/b.spec.ts'), 'status': 'failed', 'message': 'load error', 'assertionResults': []},
                {'name': str(root / 'src/c.spec.ts'), 'status': 'passed', 'assertionResults': []}]}
            path = Path(tmp) / 'vitest.json'; path.write_text(json.dumps(report))
            failed, _ = baseline.vitest_failures(path, root)
            self.assertEqual(failed, {'src/a.spec.ts > A > breaks', 'src/b.spec.ts [file]'})

    def compare(self, current, upstream):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            (tmp / 'current').write_text(''.join(f'{key}\n' for key in current))
            (tmp / 'baseline').write_text(''.join(f'{key}\n' for key in upstream))
            env = {key: value for key, value in os.environ.items() if key != 'GITHUB_STEP_SUMMARY'}
            result = subprocess.run([sys.executable, str(SCRIPT), 'compare', '--current', str(tmp / 'current'), '--baseline', str(tmp / 'baseline'), '--label', 'frontend', '--inherited', str(tmp / 'inherited')], text=True, capture_output=True, env=env)
            inherited = (tmp / 'inherited').read_text() if (tmp / 'inherited').exists() else ''
            return result, inherited

    def test_inherited_failures_pass_and_are_recorded(self):
        result, inherited = self.compare(['src/a.spec.ts > A > breaks'], ['src/a.spec.ts > A > breaks', 'other'])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(inherited, 'frontend: src/a.spec.ts > A > breaks\n')
        self.assertIn('::warning::', result.stdout)

    def test_new_failure_fails(self):
        result, _ = self.compare(['src/a.spec.ts > A > breaks', 'jack regression'], ['src/a.spec.ts > A > breaks'])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('::error::New test failure (passes on upstream): jack regression', result.stdout)

    def test_unidentified_failure_fails(self):
        result, _ = self.compare([], [])
        self.assertNotEqual(result.returncode, 0)

if __name__ == '__main__': unittest.main()
