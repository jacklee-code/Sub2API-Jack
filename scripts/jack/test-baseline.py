#!/usr/bin/env python3
"""Compare Jack test failures with the official upstream commit Jack is based on.

Jack checks fail only on failures the recorded upstream commit does not also have,
so a broken official release does not stop synchronization or publication. Lint,
type-check and build steps stay strict; only individual test failures are compared.
"""
import argparse
import json
import os
from pathlib import Path
import sys

def go_failures(paths):
    failed, failed_packages, output = set(), set(), {}
    for path in paths:
        for line in Path(path).read_text(encoding='utf-8', errors='replace').splitlines():
            try: event = json.loads(line)
            except ValueError: continue
            package, test = event.get('Package', ''), event.get('Test')
            key = f'{package} {test}' if test else f'{package} [package]'
            if event.get('Action') == 'output': output.setdefault(key, []).append(event.get('Output', ''))
            elif event.get('Action') == 'fail':
                if test: failed.add(key)
                else: failed_packages.add(package)
    # A package fails whenever one of its tests does; report it only on its own
    # (build, vet or TestMain failures).
    tested = {key.split(' ', 1)[0] for key in failed}
    failed |= {f'{package} [package]' for package in failed_packages - tested}
    return failed, output

def vitest_failures(path, root):
    failed, output = set(), {}
    for result in json.loads(Path(path).read_text(encoding='utf-8')).get('testResults', []):
        name = os.path.relpath(result['name'], root).replace(os.sep, '/')
        assertions = [a for a in result.get('assertionResults', []) if a.get('status') == 'failed']
        for assertion in assertions:
            key = f"{name} > {assertion['fullName']}"
            failed.add(key); output[key] = assertion.get('failureMessages', [])
        if result.get('status') == 'failed' and not assertions:
            key = f'{name} [file]'
            failed.add(key); output[key] = [result.get('message', '')]
    return failed, output

def collect(args):
    if args.kind == 'go': failed, output = go_failures(args.files)
    else: failed, output = vitest_failures(args.files[0], args.root)
    for key in sorted(failed):
        print(f'--- {key}', *output.get(key, [])[-40:], sep='\n', file=sys.stderr)
        print(key)

def compare(args):
    current = set(Path(args.current).read_text(encoding='utf-8').splitlines()) - {''}
    baseline = set(Path(args.baseline).read_text(encoding='utf-8').splitlines()) - {''}
    new, inherited = sorted(current - baseline), sorted(current & baseline)
    summary = [f'### {args.label} tests compared with upstream {args.upstream}', '']
    summary += [f'- New failure: `{key}`' for key in new]
    summary += [f'- Inherited from upstream: `{key}`' for key in inherited]
    if os.environ.get('GITHUB_STEP_SUMMARY'):
        with open(os.environ['GITHUB_STEP_SUMMARY'], 'a', encoding='utf-8') as f: f.write('\n'.join(summary) + '\n')
    for key in inherited: print(f'::warning::Inherited upstream test failure: {key}')
    for key in new: print(f'::error::New test failure (passes on upstream): {key}')
    if args.inherited and inherited:
        Path(args.inherited).write_text(''.join(f'{args.label}: {key}\n' for key in inherited), encoding='utf-8')
    if not current: raise SystemExit('Tests failed but no failing test could be identified')
    if new: raise SystemExit(1)

def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('collect', help='print failing test IDs from go test -json or vitest JSON output')
    p.add_argument('kind', choices=['go', 'vitest'])
    p.add_argument('files', nargs='+')
    p.add_argument('--root', default='.', help='directory vitest file names are made relative to')
    p.set_defaults(func=collect)
    p = sub.add_parser('compare', help='fail on failures that are not in the upstream baseline')
    p.add_argument('--current', required=True)
    p.add_argument('--baseline', required=True)
    p.add_argument('--label', required=True)
    p.add_argument('--upstream', default='baseline')
    p.add_argument('--inherited', help='write inherited failures here for release notes')
    p.set_defaults(func=compare)
    args = parser.parse_args()
    args.func(args)

if __name__ == '__main__': main()
