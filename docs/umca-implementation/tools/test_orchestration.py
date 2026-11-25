#!/usr/bin/env python3
"""Local Git protocol fixtures; not an autonomous runner or crash-durability test."""
from pathlib import Path
import subprocess
import tempfile
import unittest


class PromotionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'source'
        self.repo.mkdir()
        self.git(self.repo, 'init', '-q')
        self.git(self.repo, 'config', 'user.email', 'fixture@example.invalid')
        self.git(self.repo, 'config', 'user.name', 'Fixture')
        (self.repo / 'base').write_text('native\n')
        self.git(self.repo, 'add', '.')
        self.git(self.repo, 'commit', '-qm', 'baseline')
        self.base = self.git(self.repo, 'rev-parse', 'HEAD')
        self.accepted = 'refs/heads/umca/impl/fixture/integration'
        self.git(self.repo, 'update-ref', self.accepted, self.base, '0' * 40)

    def git(self, path, *args):
        return subprocess.check_output(['git', '-C', str(path), *args], stderr=subprocess.PIPE, text=True).strip()

    def branch(self, name, base, changes):
        path = self.root / name
        self.git(self.repo, 'worktree', 'add', '-b', name, str(path), base)
        for file, data in changes.items():
            (path / file).write_text(data)
        self.git(path, 'add', '.')
        self.git(path, 'commit', '-qm', name)
        return self.git(path, 'rev-parse', 'HEAD')

    def trial(self, name, parent, candidate):
        path = self.root / name
        self.git(self.repo, 'worktree', 'add', '-b', name, str(path), parent)
        self.git(path, 'merge', '--no-ff', '-m', 'trial', candidate)
        return self.git(path, 'rev-parse', 'HEAD')

    def test_reject_trial_retry_complete_diff(self):
        candidate = self.branch('candidate-1', self.base, {'feature': 'broken\n', 'required': 'keep\n'})
        rejected = self.trial('trial-1', self.base, candidate)
        self.assertEqual(self.git(self.repo, 'rev-parse', self.accepted), self.base)
        repaired = self.branch('candidate-2', self.base, {'feature': 'fixed\n', 'required': 'keep\n'})
        trial = self.trial('trial-2', self.base, repaired)
        self.assertEqual(self.git(self.repo, 'rev-parse', trial+'^1'), self.base)
        self.assertEqual(self.git(self.repo, 'rev-parse', trial+'^2'), repaired)
        self.git(self.repo, 'update-ref', self.accepted, trial, self.base)
        self.assertEqual(self.git(self.repo, 'show', self.accepted+':required'), 'keep')
        self.assertEqual(self.git(self.repo, 'show', self.accepted+':feature'), 'fixed')
        with self.assertRaises(subprocess.CalledProcessError):
            self.git(self.repo, 'merge-base', '--is-ancestor', rejected, self.accepted)

    def test_stale_promotion_cannot_overwrite_accepted_ref(self):
        c1 = self.branch('candidate-a', self.base, {'a': 'a\n'})
        c2 = self.branch('candidate-b', self.base, {'b': 'b\n'})
        t1 = self.trial('trial-a', self.base, c1)
        t2 = self.trial('trial-b', self.base, c2)
        self.git(self.repo, 'update-ref', self.accepted, t1, self.base)
        with self.assertRaises(subprocess.CalledProcessError):
            self.git(self.repo, 'update-ref', self.accepted, t2, self.base)
        self.assertEqual(self.git(self.repo, 'rev-parse', self.accepted), t1)


if __name__ == '__main__':
    unittest.main()
