#!/usr/bin/env python3
"""No-network fixtures; create local bare cache with Git and verify its identity."""
import copy
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from acquire_references import verify, validate, acquire, validate_root


class ReferenceTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name); self.repo=self.root/'work';self.repo.mkdir()
        def g(*args):return subprocess.check_output(['git','-C',str(self.repo),*args]).decode().strip()
        g('init','-q');g('config','user.name','Fixture');g('config','user.email','fixture@example.invalid')
        data=b'package example\n'; (self.repo/'example.go').write_bytes(data)
        g('add','.');g('commit','-qm','fixture')
        commit=g('rev-parse','HEAD');tree=g('rev-parse','HEAD^{tree}')
        self.e={'id':'example','repository':'example/example','url':'https://github.com/example/example.git','commit':commit,'tree':tree,'roles':['HST'],'files':{'example.go':{'blob_sha1':hashlib.sha1(f'blob {len(data)}\0'.encode()+data).hexdigest(),'lines':1}}}
        self.cache=self.root/'example.git';subprocess.check_call(['git','clone','--bare','-q',str(self.repo),str(self.cache)])
    def test_root_rejects_git_directory_before_creation(self):
        nested = self.repo / '.git' / 'objects' / 'unexpected-cache'
        with self.assertRaises(ValueError): validate_root(nested)
        self.assertFalse(nested.exists())
        with self.assertRaises(ValueError): validate_root(self.repo / 'nested')
        with self.assertRaises(ValueError): validate_root(self.cache / 'nested')

    def test_new_cache_via_controlled_local_fetch(self):
        from unittest.mock import patch
        import acquire_references as ar
        real = ar.git
        e = copy.deepcopy(self.e)
        e['id'] = 'newcache'
        def fixture_git(*args, timeout=120):
            if 'fetch' in args:
                # Substitute a local fixture URL ONLY inside this test. Keep
                # the production fetch path and publication logic intact.
                args = tuple(str(self.repo) if x == e['url'] else x for x in args)
                import os
                env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull)
                return subprocess.check_output(['git', '-c','protocol.file.allow=always', *map(str,args)], env=env, stderr=subprocess.PIPE, timeout=timeout)
            return real(*args, timeout=timeout)
        with patch.object(ar, 'git', side_effect=fixture_git):
            receipt = acquire(self.root, e)
        self.assertEqual(receipt['status'], 'VERIFIED')
        self.assertEqual(verify(self.root/'newcache.git',e)['verified_files'],1)
        self.assertEqual(real('--git-dir',self.root/'newcache.git','rev-parse','refs/umca/pinned').decode().strip(),e['commit'])

    def test_invalid_manifest_paths_and_empty_inventory(self):
        for field, value in [('repository','../example'),('files',{}),('roles',[])]:
            e=copy.deepcopy(self.e);e[field]=value
            if field == 'repository': e['url']='https://github.com/'+value+'.git'
            with self.assertRaises(ValueError):validate({'schema_version':1,'repositories':[e]})
        e=copy.deepcopy(self.e);e['files']['a//b']=e['files']['example.go']
        with self.assertRaises(ValueError):validate({'schema_version':1,'repositories':[e]})

    def test_exact_cache(self): self.assertEqual(acquire(self.root,self.e)['verified_files'],1)
    def test_wrong_tree(self):
        e=copy.deepcopy(self.e);e['tree']='0'*40
        with self.assertRaises(ValueError): verify(self.cache,e)
    def test_wrong_blob(self):
        e=copy.deepcopy(self.e);e['files']['example.go']['blob_sha1']='0'*40
        with self.assertRaises(ValueError): verify(self.cache,e)
    def test_non_bare(self):
        with self.assertRaises((ValueError,subprocess.CalledProcessError)): verify(self.repo,e=self.e)
    def test_bad_url_or_id(self):
        for key,value in [('id','../outside'),('url','file:///tmp/a'),('commit','HEAD')]:
            e=copy.deepcopy(self.e);e[key]=value
            with self.assertRaises(ValueError):validate({'schema_version':1,'repositories':[e]})
    def test_manifest(self):self.assertEqual(len(validate({'schema_version':1,'repositories':[self.e]})),1)

if __name__=='__main__':unittest.main()
