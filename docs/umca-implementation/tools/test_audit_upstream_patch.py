#!/usr/bin/env python3
"""Disposable Git fixture tests for the mechanical audit helper, no Crush code."""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name('audit_upstream_patch.py')


class AuditTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        self.git('init', '-q')
        self.git('config', 'user.email', 'fixture@example.invalid')
        self.git('config', 'user.name', 'Fixture')
        self.write('existing.go', 'package p\n\nfunc f() {\n\told()\n}\n')
        self.write('schema.json', '{"a": 1}\n')
        self.base = self.commit()

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.repo), *args]).decode().strip()

    def write(self, path, data):
        (self.repo / path).write_text(data)

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')
        return self.git('rev-parse', 'HEAD')

    def audit(self, exceptions=None):
        cmd = [sys.executable, str(SCRIPT), '--repo', str(self.repo), '--base', self.base]
        if exceptions is not None:
            # Outside tracked fixture path: pass file via separate temporary directory.
            with tempfile.TemporaryDirectory() as d:
                p = Path(d) / 'exceptions.json'
                p.write_text(json.dumps(exceptions))
                r = subprocess.run(cmd + ['--exceptions', str(p)], capture_output=True, text=True)
        else:
            r = subprocess.run(cmd, capture_output=True, text=True)
        self.assertIn(r.returncode, (0, 1), r.stderr)
        return r.returncode, json.loads(r.stdout)

    def test_malformed_html_and_line_comment(self):
        for bad in ('<!-- UMCA-BEGIN: HST.call.', '// UMCA-BEGIN: HST.call. -->'):
            self.write('existing.go', 'package p\n'+bad+'\nnewCall()\n// UMCA-END: HST.call.\n')
            self.commit()
            status, report = self.audit()
            self.assertEqual(status, 1)
            self.assertTrue(report['inherited_paths'][0]['marker_errors'])

    def test_mixed_comment_styles(self):
        self.write('existing.go', 'package p\n// UMCA-BEGIN: HST.call.\nnewCall()\n# UMCA-END: HST.call.\n')
        self.commit()
        code, report = self.audit()
        self.assertEqual(code, 1)
        self.assertTrue(report['inherited_paths'][0]['marker_errors'])

    def test_replacement_ref_cannot_hide_unmarked_change(self):
        self.write('existing.go', 'package p\nfunc changed() {}\n')
        bad = self.commit()
        # A later tree restores the base. Replacing the bad commit's object
        # with this commit would otherwise hide the entire unmarked diff.
        self.write('existing.go', 'package p\n\nfunc f() {\n\told()\n}\n')
        clean = self.commit()
        self.git('replace', bad, clean)
        r = subprocess.run([sys.executable, str(SCRIPT), '--repo', str(self.repo),
                            '--base', self.base, '--head', bad], capture_output=True, text=True)
        self.assertEqual(r.returncode, 1, r.stderr)
        self.assertEqual(json.loads(r.stdout)['head'], bad)

    def test_ambient_git_dir_does_not_redirect_audit(self):
        import os
        self.write('existing.go', 'package changed\n')
        self.commit()
        with tempfile.TemporaryDirectory() as other:
            subprocess.check_call(['git','init','-q',other])
            env = dict(os.environ, GIT_DIR=str(Path(other)/'.git'))
            r = subprocess.run([sys.executable, str(SCRIPT), '--repo', str(self.repo),
                                '--base', self.base], env=env, capture_output=True, text=True)
            self.assertEqual(r.returncode, 1, r.stderr)

    def test_marked_and_new(self):
        self.write('existing.go', 'package p\n\nfunc f() {\n\t// UMCA-BEGIN: HST.call.\n\tnewCall()\n\t// UMCA-END: HST.call.\n}\n')
        self.write('umca_new.go', 'package p\n')
        self.commit()
        status, report = self.audit()
        self.assertEqual(status, 0, report)
        self.assertEqual(report['new_paths'], ['umca_new.go'])
        self.assertEqual(report['summary']['marker_blocks'], 1)

    def test_unmarked(self):
        self.write('existing.go', 'package p\n\nfunc f() {\n\tnewCall()\n}\n')
        self.commit()
        self.assertEqual(self.audit()[0], 1)

    def test_malformed(self):
        self.write('existing.go', 'package p\n\nfunc f() {\n\t// UMCA-BEGIN: HST.call.\n\tnewCall()\n\t// UMCA-END: HST.other.\n}\n')
        self.commit()
        self.assertTrue(self.audit()[1]['inherited_paths'][0]['marker_errors'])

    def test_deletion_anchor(self):
        self.write('existing.go', 'package p\n\nfunc f() {\n\t// UMCA-BEGIN: HST.removed.\n\t// UMCA-END: HST.removed.\n}\n')
        self.commit()
        self.assertEqual(self.audit()[0], 0)

    def test_unmarked_deletion(self):
        self.write('existing.go', 'package p\n\nfunc f() {\n}\n')
        self.commit()
        self.assertEqual(self.audit()[0], 1)

    def test_exception_and_stale_hash(self):
        self.write('schema.json', '{"a": 2}\n')
        self.commit()
        status, report = self.audit()
        self.assertEqual(status, 1)
        evidence = report['inherited_paths'][0]['proposed_exception']
        exc = dict(evidence, id='OPS.schema.generated', kind='commentless', reason='fixture JSON',
                   owner='OPS', reviewer='AUD', evidence='fixture review')
        self.assertEqual(self.audit([exc])[0], 0)
        self.write('schema.json', '{"a": 3}\n')
        self.commit()
        self.assertEqual(self.audit([exc])[0], 1)

    def test_file_deletion_requires_exception(self):
        (self.repo / 'existing.go').unlink()
        self.commit()
        code, result = self.audit()
        self.assertEqual(code, 1)
        self.assertIn('deleted-file', result['inherited_paths'][0]['structural'])

    def test_pure_deletion_inside_existing_region(self):
        self.write('existing.go', 'package p\n\nfunc f() {\n\t// UMCA-BEGIN: HST.call.\n\told()\n\t// UMCA-END: HST.call.\n}\n')
        self.commit()
        self.base = self.git('rev-parse', 'HEAD')
        self.write('existing.go', 'package p\n\nfunc f() {\n\t// UMCA-BEGIN: HST.call.\n\t// UMCA-END: HST.call.\n}\n')
        self.commit()
        self.assertEqual(self.audit()[0], 0)

    def test_mixed_uncovered(self):
        self.write('existing.go', 'package q\n\nfunc f() {\n\t// UMCA-BEGIN: HST.call.\n\tnewCall()\n\t// UMCA-END: HST.call.\n}\n')
        self.commit()
        self.assertEqual(self.audit()[0], 1)


if __name__ == '__main__':
    unittest.main()
