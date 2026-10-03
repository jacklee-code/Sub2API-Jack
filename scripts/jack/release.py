#!/usr/bin/env python3
"""Reproducible Jack release metadata and immutable binary/image build inputs."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[2]
VERSION = re.compile(r'(\d+\.\d+\.\d+)-jack\.([1-9]\d*)\Z')

def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, text=True).strip()

def runtime_id():
    paths = [ROOT / 'Dockerfile.goreleaser', ROOT / 'deploy/docker-entrypoint.sh', ROOT / 'deploy/jack/entrypoint.sh']
    paths += sorted(p for p in (ROOT / 'backend/resources').rglob('*') if p.is_file())
    h = hashlib.sha256()
    for path in paths:
        h.update(path.relative_to(ROOT).as_posix().encode() + b'\0' + path.read_bytes() + b'\0')
    return h.hexdigest()

def metadata(version):
    match = VERSION.fullmatch(version)
    if not match:
        raise ValueError('expected X.Y.Z-jack.N')
    upstream = json.loads((ROOT / '.jack/upstream.json').read_text())
    if upstream['tag'] != 'v' + match[1]:
        raise ValueError('version does not match recorded upstream')
    subprocess.run(['git', 'merge-base', '--is-ancestor', upstream['commit'], 'HEAD'], cwd=ROOT, check=True)
    return {'format': 1, 'version': version, 'upstream_version': match[1], 'upstream_commit': upstream['commit'], 'runtime_id': runtime_id(), 'schema_epoch': 1}

def next_version():
    base = json.loads((ROOT / '.jack/upstream.json').read_text())['tag'].removeprefix('v')
    values = [int(m[2]) for tag in git('tag', '--list', f'v{base}-jack.*').splitlines() if (m := VERSION.fullmatch(tag.removeprefix('v')))]
    return f'{base}-jack.{max(values, default=0) + 1}'

def package(version, arch, binary):
    info = metadata(version)
    output = ROOT / '.jack-dist'
    output.mkdir(exist_ok=True)
    (output / 'jack-release.json').write_text(json.dumps(info, indent=2) + '\n')
    with tarfile.open(output / f'sub2api_{version}_linux_{arch}.tar.gz', 'w:gz') as archive:
        archive.add(binary, arcname='sub2api')
        archive.add(ROOT / 'LICENSE', arcname='LICENSE')
    context = ROOT / '.jack-build' / arch
    context.mkdir(parents=True, exist_ok=True)
    shutil.copy2(binary, context / 'sub2api')
    for folder in ['deploy', 'backend/resources']:
        shutil.copytree(ROOT / folder, context / folder, dirs_exist_ok=True)
    dockerfile = (ROOT / 'Dockerfile.goreleaser').read_text()
    dockerfile += '\nCOPY deploy/jack/entrypoint.sh /app/jack-entrypoint.sh\nRUN chmod 755 /app/jack-entrypoint.sh\nENTRYPOINT ["/app/jack-entrypoint.sh"]\nCMD []\n'
    (context / 'Dockerfile').write_text(dockerfile)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['next-version', 'runtime-id', 'manifest', 'package', 'checksums'])
    parser.add_argument('--version')
    parser.add_argument('--arch', choices=['arm64', 'amd64'])
    parser.add_argument('--binary')
    args = parser.parse_args()
    if args.command == 'next-version': print(next_version())
    elif args.command == 'runtime-id': print(runtime_id())
    elif args.command == 'manifest': print(json.dumps(metadata(args.version), indent=2))
    elif args.command == 'package': package(args.version, args.arch, args.binary)
    else:
        root = ROOT / '.jack-dist'
        files = sorted(root.glob('sub2api_*.tar.gz')) + [root / 'jack-release.json']
        (root / 'checksums.txt').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in files))

if __name__ == '__main__': main()
