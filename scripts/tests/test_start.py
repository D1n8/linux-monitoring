"""Launcher regression checks; no Docker daemon, networking, or host mutation."""
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
FAKE = '''#!/usr/bin/env python3
import json,os,pathlib,sys
args=sys.argv[1:];root=pathlib.Path(os.environ['FIXTURE'])
with (root/'calls').open('a') as f:f.write(json.dumps(args)+'\\n')
if args[:2]==['compose','version']:print('5.6.0')
elif args==['info','--format','{{.Architecture}}']:print('x86_64')
elif args==['info']:pass
elif args[:2]==['image','inspect']:sys.exit(0 if (root/'image-present').exists() else 1)
elif args[:2]==['image','load']:(root/'image-present').touch()
elif args[:3]==['compose','config','--images']:print('ghcr.io/d1n8/goinmetrics:latest\\npostgres:17-alpine')
elif args[0]=='compose' and 'build' in args and '--build' not in args:(root/'image-present').touch()
elif args[:2]==['compose','up']:
 if '--no-build' not in args or '--build' in args:sys.exit(88)
 if os.environ.get('FAIL_UP'):sys.exit(1)
else:sys.exit(89)
'''

class LauncherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(); self.addCleanup(self.temp.cleanup)
        self.path = pathlib.Path(self.temp.name)
        for filename in ['start.sh','compose.yaml','compose.build.yaml','Dockerfile']:
            shutil.copy(ROOT / filename, self.path / filename)
        (self.path / '.env').write_text('POSTGRES_PASSWORD=' + 'a'*64 + '\n')
        binary = self.path / 'docker'; binary.write_text(FAKE); binary.chmod(0o755)
        self.env = {**os.environ, 'PATH': str(self.path)+os.pathsep+os.environ['PATH'], 'FIXTURE': str(self.path)}
    def run_start(self, *args):
        return subprocess.run(['bash', str(self.path/'start.sh'), *args], env=self.env, capture_output=True, text=True)
    def calls(self):
        return [json.loads(line) for line in (self.path/'calls').read_text().splitlines()]
    def add_image_archive(self, valid=True):
        archive=self.path/'goinmetrics-image.tar.gz';archive.write_bytes(b'prebuilt image fixture')
        checksum=hashlib.sha256(archive.read_bytes()).hexdigest() if valid else '0'*64
        (self.path/'SHA256SUMS').write_text(checksum+'  goinmetrics-image.tar.gz\n')
    def test_default_pulls_ready_images_without_building(self):
        result=self.run_start();self.assertEqual(result.returncode,0,result.stderr)
        self.assertFalse(any('build' in call for call in self.calls()))
        self.assertTrue(any(call[:2]==['compose','up'] and '--no-build' in call for call in self.calls()))
    def test_unavailable_ready_image_never_silently_compiles(self):
        self.env['FAIL_UP']='1';result=self.run_start();self.assertNotEqual(result.returncode,0)
        self.assertFalse(any('build' in call for call in self.calls()))
    def test_build_requires_explicit_flag(self):
        result=self.run_start('--build');self.assertEqual(result.returncode,0,result.stderr)
        self.assertTrue(any('build' in call for call in self.calls()))
    def test_bundle_is_verified_and_loaded(self):
        self.add_image_archive();result=self.run_start();self.assertEqual(result.returncode,0,result.stderr)
        self.assertTrue(any(call[:2]==['image','load'] for call in self.calls()))
    def test_corrupt_bundle_is_rejected_before_loading_or_starting(self):
        self.add_image_archive(False);result=self.run_start();self.assertNotEqual(result.returncode,0)
        self.assertFalse(any(call[:2] in [['image','load'],['compose','up']] for call in self.calls()))
    def test_bundle_for_wrong_architecture_is_rejected(self):
        (self.path/'PLATFORM').write_text('linux/arm64\n');result=self.run_start();self.assertNotEqual(result.returncode,0)
        self.assertFalse(any(call[:2]==['compose','up'] for call in self.calls()))
    def test_cached_image_does_not_reload_archive(self):
        (self.path/'image-present').touch();self.add_image_archive(False)
        result=self.run_start();self.assertEqual(result.returncode,0,result.stderr)
        self.assertFalse(any(call[:2]==['image','load'] for call in self.calls()))

if __name__ == '__main__':unittest.main()
