#!/usr/bin/env python3
"""Merge official stable releases into a reviewable branch; never rewrite main."""
import argparse
import fnmatch
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
UPSTREAM = 'Wei-Shaw/sub2api'
REPO = 'jacklee-code/Sub2API-Jack'
OWNER = 'jacklee-code'

def run(*args, check=True):
    return subprocess.run(args, cwd=ROOT, text=True, capture_output=True, check=check)

def output(**values):
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as f:
            for key, value in values.items(): f.write(f'{key}={value}\n')
    print(json.dumps(values))

def load_policy():
    path = ROOT / '.jack/merge-policy.json'
    policy = json.loads(path.read_text()) if path.exists() else {}
    return {'upstream_wins': policy.get('upstream_wins', []), 'regenerate': policy.get('regenerate', {}), 'union': policy.get('union', [])}

def strategy(path, policy):
    if any(fnmatch.fnmatchcase(path, pattern) for pattern in policy['upstream_wins']): return 'upstream'
    if path in policy['union']: return 'union'
    if path in policy['regenerate']: return 'regenerate'
    return None

def resolve_conflicts(conflicts, policy):
    """Resolve conflicts covered by .jack/merge-policy.json; return the resolved paths.

    upstream: take the official version (Jack keeps no edits there).
    union: keep every line of both sides (go.sum; CI proves the module graph).
    regenerate: keep Jack's copy and rebuild it from the merged manifests; Jack
    checks install with --frozen-lockfile, so a stale result fails before merge.
    """
    plan = {path: strategy(path, policy) for path in conflicts}
    if not conflicts or None in plan.values(): return None
    for path, how in sorted(plan.items(), key=lambda item: item[1] == 'regenerate'):
        if how == 'upstream':
            if run('git', 'checkout', '--theirs', '--', path, check=False).returncode != 0:
                if run('git', 'rm', '-q', '--', path, check=False).returncode != 0: return None
                continue
        elif how == 'union':
            sides = [run('git', 'show', f':{stage}:{path}', check=False) for stage in (2, 3)]
            if any(side.returncode != 0 for side in sides): return None
            lines = {line for side in sides for line in side.stdout.splitlines() if line}
            (ROOT / path).write_text(''.join(f'{line}\n' for line in sorted(lines)))
        else:
            run('git', 'checkout', '--ours', '--', path)
            regenerate = run(*policy['regenerate'][path], check=False)
            if regenerate.returncode != 0:
                print(regenerate.stdout, regenerate.stderr); return None
        run('git', 'add', '--', path)
    if run('git', 'diff', '--name-only', '--diff-filter=U').stdout.strip(): return None
    return plan

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    release = json.loads(run('gh', 'api', f'repos/{UPSTREAM}/releases/latest').stdout)
    tag = release['tag_name']
    if release.get('draft') or release.get('prerelease') or not re.fullmatch(r'v\d+\.\d+\.\d+', tag):
        raise SystemExit('Refusing an unofficial release version')
    current = json.loads((ROOT / '.jack/upstream.json').read_text())
    latest_tuple = tuple(map(int, tag[1:].split('.')))
    current_tuple = tuple(map(int, current['tag'][1:].split('.')))
    if latest_tuple <= current_tuple:
        output(ready='false', reason='already_current', tag=tag); return
    if args.dry_run:
        output(ready='false', reason='dry_run', current=current['tag'], target=tag); return
    if run('git', 'status', '--porcelain').stdout.strip():
        raise SystemExit('Working tree must be clean')
    run('git', 'fetch', 'origin', 'main')
    run('git', 'fetch', f'https://github.com/{UPSTREAM}.git', f'refs/tags/{tag}')
    commit = run('git', 'rev-parse', 'FETCH_HEAD^{commit}').stdout.strip()
    branch = f'sync/upstream-{tag}'
    existing = json.loads(run('gh', 'pr', 'list', '--repo', REPO, '--head', branch, '--state', 'open', '--json', 'number,isDraft,headRefOid').stdout)
    if existing:
        pr = existing[0]
        # An open failed/conflicting PR is for a human/agent to repair. Do not
        # repeatedly replace it, erase repairs, or publish an unreviewed base.
        output(ready='false', reason='existing_pr', pr=pr['number'], blocked=str(pr['isDraft']).lower()); return
    run('git', 'switch', '-c', branch, 'origin/main')
    policy = load_policy()
    merge = run('git', 'merge', '--no-ff', '--no-edit', commit, check=False)
    blocked, resolved = merge.returncode != 0, {}
    if blocked:
        conflicts = run('git', 'diff', '--name-only', '--diff-filter=U').stdout
        resolved = resolve_conflicts(conflicts.split(), policy)
        blocked = resolved is None
    if blocked:
        run('git', 'merge', '--abort')
        (ROOT / '.jack/upstream-conflict.md').write_text(f'# Upstream {tag} needs manual integration\n\nTarget commit: `{commit}`\n\nConflicting paths:\n\n```\n{conflicts}```\n\nMerge the target, remove this file, update upstream.json and rerun Jack checks.\n')
        run('git', 'add', '.jack/upstream-conflict.md')
        run('git', 'commit', '-m', f'chore: record blocked upstream {tag} integration')
    else:
        if merge.returncode != 0: run('git', 'commit', '--no-edit')
        (ROOT / '.jack/upstream.json').write_text(json.dumps({'tag': tag, 'commit': commit}, indent=2) + '\n')
        run('git', 'add', '.jack/upstream.json')
        run('git', 'commit', '-m', f'chore: track upstream {tag}')
    run('git', 'push', 'origin', branch)
    body = f'Integrate official [{tag}]({release["html_url"]}) ({commit}) while retaining Jack features.\n\n'
    if resolved:
        body += 'Conflicts resolved by `.jack/merge-policy.json`:\n' + ''.join(f'- `{path}`: {how}\n' for path, how in sorted(resolved.items())) + '\n'
    body += 'Blocked by merge conflicts; this draft must not be merged until the integration is repaired.' if blocked else 'Jack checks must pass before automatic merge and publication. Production deployment remains manual.'
    with tempfile.NamedTemporaryFile('w', suffix='.md') as f:
        f.write(body); f.flush()
        command = ['gh', 'pr', 'create', '--repo', REPO, '--base', 'main', '--head', branch, '--title', f'chore: integrate upstream {tag}', '--body-file', f.name, '--assignee', OWNER]
        if blocked: command.append('--draft')
        url = run(*command).stdout.strip()
    output(ready=str(not blocked).lower(), blocked=str(blocked).lower(), sha=run('git', 'rev-parse', 'HEAD').stdout.strip(), pr=url, tag=tag)

if __name__ == '__main__': main()
