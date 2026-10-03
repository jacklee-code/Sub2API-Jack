import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

class EntrypointTest(unittest.TestCase):
    @unittest.skipUnless(shutil.which('docker') and os.getuid() != 0, 'requires Docker and a non-root test user')
    def test_container_recreation_preserves_installed_binary(self):
        entrypoint = Path(__file__).resolve().parents[2] / 'deploy/jack/entrypoint.sh'
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            data = root / 'data'; data.mkdir()
            bundled = root / 'sub2api'
            bundled.write_text('#!/bin/sh\necho bundled\n'); bundled.chmod(0o755)
            command = ['docker','run','--rm','--network','none','--user',f'{os.getuid()}:{os.getgid()}',
                       '-v',f'{data}:/app/data','-v',f'{bundled}:/app/sub2api:ro',
                       '-v',f'{entrypoint}:/entrypoint:ro','alpine:3.21','sh','/entrypoint']
            self.assertEqual(subprocess.check_output(command,text=True).strip(), 'bundled')
            installed = data / 'jack-runtime/sub2api'
            installed.write_text('#!/bin/sh\necho installed-update\n'); installed.chmod(0o755)
            self.assertEqual(subprocess.check_output(command,text=True).strip(), 'installed-update')
            self.assertEqual(bundled.read_text(), '#!/bin/sh\necho bundled\n')

if __name__ == '__main__': unittest.main()
