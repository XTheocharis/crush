#!/usr/bin/env python3
"""Acquire and verify pinned reference Git objects; no checkout or build."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tempfile

SHA = re.compile(r'[0-9a-f]{40}\Z')
ID = re.compile(r'[a-zA-Z0-9][a-zA-Z0-9_-]*\Z')
REPO = re.compile(r'[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+\Z')


def git(*args, timeout=120):
    # Do not use Git URL rewrite, remote helpers, hooks or local/global custom
    # clone templates from user config for third-party reference acquisition.
    env = os.environ.copy()
    for k in list(env):
        if k.startswith('GIT_') and k not in ('GIT_SSL_CAINFO', 'GIT_SSL_CAPATH'):
            env.pop(k)
    env.update(GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
               GIT_TERMINAL_PROMPT='0', GIT_NO_REPLACE_OBJECTS='1',
               GIT_LFS_SKIP_SMUDGE='1')
    return subprocess.check_output(['git', '-c', 'core.hooksPath='+os.devnull,
                                    '-c', 'protocol.ext.allow=never',
                                    '-c', 'protocol.file.allow=never', *map(str,args)],
                                   env=env, stderr=subprocess.PIPE, timeout=timeout)


def validate(m):
    if m.get('schema_version') != 1 or not isinstance(m.get('repositories'),list):
        raise ValueError('unsupported manifest')
    seen = set()
    for e in m['repositories']:
        if not ID.fullmatch(e['id']) or e['id'] in seen:
            raise ValueError('invalid/duplicate repository ID')
        seen.add(e['id'])
        if not REPO.fullmatch(e['repository']) or any(part in ('.', '..') for part in e['repository'].split('/')) or e['url'] != 'https://github.com/'+e['repository']+'.git':
            raise ValueError('invalid canonical GitHub URL')
        if not SHA.fullmatch(e['commit']) or not SHA.fullmatch(e['tree']):
            raise ValueError('invalid Git SHA-1 pin')
        if not isinstance(e.get('roles'), list) or not e['roles'] or not all(isinstance(r, str) and r for r in e['roles']):
            raise ValueError('invalid role list')
        if not isinstance(e.get('files'), dict) or not e['files']:
            raise ValueError('empty or invalid file inventory')
        for path, f in e['files'].items():
            p = PurePosixPath(path)
            if not path or p.is_absolute() or p.as_posix() != path or '..' in p.parts or '\\' in path or '\0' in path or not SHA.fullmatch(f['blob_sha1']):
                raise ValueError('invalid file or blob pin')
    return m['repositories']


def verify(path, e, timeout=120):
    if path.is_symlink() or not path.is_dir():
        raise ValueError('reference cache must be a regular directory, not symlink')
    def g(*args): return git('--git-dir',path,*args,timeout=timeout)
    if g('rev-parse','--is-bare-repository').strip()!=b'true':
        raise ValueError('reference cache is not bare')
    if g('rev-parse','--show-object-format').strip()!=b'sha1':
        raise ValueError('unexpected Git object format')
    if g('rev-parse','--verify',e['commit']+'^{commit}').decode().strip()!=e['commit']:
        raise ValueError('commit mismatch')
    if g('rev-parse',e['commit']+'^{tree}').decode().strip()!=e['tree']:
        raise ValueError('tree mismatch')
    for kind, oid in (('commit', e['commit']), ('tree', e['tree'])):
        data = g('cat-file', kind, oid)
        if hashlib.sha1(f'{kind} {len(data)}\0'.encode()+data).hexdigest() != oid:
            raise ValueError(kind+' bytes mismatch')
    entries={}
    for line in g('ls-tree','-rz','--full-tree',e['commit']).split(b'\0'):
        if not line: continue
        meta,p=line.split(b'\t',1)
        mode,typ,oid=meta.decode('ascii').split()
        entries[p.decode('utf-8','surrogateescape')]=(mode,typ,oid)
    for p,f in e['files'].items():
        if p not in entries or entries[p][0] not in ('100644','100755') or entries[p][1:]!=('blob',f['blob_sha1']):
            raise ValueError('indexed blob identity mismatch: '+p)
        data=g('cat-file','blob',f['blob_sha1'])
        # Rehash bytes, not just tree metadata or local object lookup.
        if hashlib.sha1(f'blob {len(data)}\0'.encode()+data).hexdigest()!=f['blob_sha1']:
            raise ValueError('blob bytes mismatch: '+p)
    return {'id':e['id'],'url':e['url'],'commit':e['commit'],'tree':e['tree'],
            'verified_files':len(e['files']),'path':str(path),'status':'VERIFIED'}


def acquire(root,e,timeout=900):
    dest=root/(e['id']+'.git')
    # ORCH serializes each ID; final rename never intentionally overwrites a cache.
    if dest.exists() or dest.is_symlink():
        return verify(dest,e,min(timeout,120))
    with tempfile.TemporaryDirectory(prefix=e['id']+'.',dir=root) as tmp:
        repo=Path(tmp)/'repo.git'
        # A narrow bare Git fetch is the clone equivalent without checking out
        # untrusted files, fetching moving branch tips or running submodules.
        git('init','--bare','--template=',repo,timeout=min(timeout,120))
        git('--git-dir',repo,'-c','protocol.https.allow=always','fetch',
            '--no-tags','--no-recurse-submodules','--depth=1',e['url'],e['commit'],timeout=timeout)
        verify(repo,e,min(timeout,120))
        git('--git-dir',repo,'update-ref','refs/umca/pinned',e['commit'],timeout=min(timeout,120))
        if dest.exists() or dest.is_symlink():
            return verify(dest,e,min(timeout,120))
        repo.rename(dest)
    return verify(dest,e,min(timeout,120))


def validate_root(root):
    # Check the nearest existing ancestor before creating directories. A path
    # under .git, a bare repo, or a linked worktree is not an external cache.
    root = root.resolve()
    parent = root
    while not parent.exists():
        parent = parent.parent
    try:
        flags = git('-C', parent, 'rev-parse', '--is-inside-work-tree',
                    '--is-inside-git-dir', '--is-bare-repository').splitlines()
    except subprocess.CalledProcessError:
        flags = []  # No discoverable repo; caller still owns the trusted root.
    if b'true' in flags:
        raise ValueError('reference cache inside a Git worktree or repository directory')
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    return root


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--manifest',required=True,type=Path)
    p.add_argument('--root',required=True,type=Path)
    p.add_argument('--id',action='append',default=[])
    p.add_argument('--role')
    p.add_argument('--list',action='store_true')
    p.add_argument('--timeout',type=int,default=900,help='bounded per-fetch timeout, seconds')
    a=p.parse_args()
    try:
        entries=validate(json.loads(a.manifest.read_text()))
        if not a.id and not a.role: raise ValueError('select --id or --role explicitly')
        if set(a.id)-{e['id'] for e in entries}: raise ValueError('unknown repository ID')
        selected=[e for e in entries if (not a.id or e['id'] in a.id) and (not a.role or a.role in e['roles'])]
        if not selected or a.timeout<=0: raise ValueError('empty selection or invalid timeout')
        if a.list:
            print(json.dumps([{k:e[k] for k in ('id','url','commit','tree','roles')} for e in selected],indent=2));return 0
        root=validate_root(a.root)
        # Run under a private owner-controlled directory; ORCH serializes each ID.
        receipts=[];failed=False
        for e in selected:
            try: receipts.append(acquire(root,e,a.timeout))
            except (OSError,ValueError,subprocess.CalledProcessError,subprocess.TimeoutExpired) as exc:
                # Do not echo raw credentials/remote stderr; GitHub URLs are public.
                receipts.append({'id':e['id'],'commit':e['commit'],'status':'BLOCKED',
                                 'error_type':type(exc).__name__,
                                 'detail':str(exc) if isinstance(exc,ValueError) else 'Git/network/filesystem acquisition failed; inspect sanitized local diagnostics'})
                failed=True
        print(json.dumps(receipts,indent=2)); return int(failed)
    except (OSError,ValueError,KeyError,TypeError,AttributeError,json.JSONDecodeError,subprocess.TimeoutExpired) as exc:
        print('ERROR: '+str(exc),file=sys.stderr);return 2


if __name__=='__main__': sys.exit(main())
