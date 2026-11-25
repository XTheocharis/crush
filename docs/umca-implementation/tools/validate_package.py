#!/usr/bin/env python3
"""Read-only offline package consistency checks, not semantic/product certification."""
import argparse
import graphlib
import hashlib
import json
from pathlib import Path
import re
import sys


def require(condition, detail):
    if not condition:
        raise ValueError(detail)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def range_set(ranges, n):
    result = set()
    for a, b in ranges:
        require(isinstance(a, int) and isinstance(b, int) and 1 <= a <= b <= n, f'invalid range {a}:{b}')
        result.update(range(a,b+1))
    return result


def validate(root, check_sums=True):
    root = root.resolve()
    source = root/'GO_AGENT_CONTEXT.md'
    data = source.read_bytes()
    lines = data.decode('utf-8').splitlines()
    index = json.loads((root/'GO_CONTEXT_INDEX.json').read_text(encoding='utf-8'))
    require(sha(data) == index['sha256'] == 'ab11c958f30ca11198b0fc5d030668eb76e48111008bc735475b5e26121a4424', 'Go context hash mismatch')
    require(len(lines) == index['lines'] == 6630 and len(index['rules']) == 248, 'Go context inventory mismatch')
    for ident, rule in index['rules'].items():
        a,b = rule['range']
        range_set([[a,b]],len(lines))
        require(lines[a-1].startswith('#### '+ident+' — '), 'rule start mismatch: '+ident)
        require(sha('\n'.join(lines[a-1:b]).encode()) == rule['text_sha256'], 'rule hash mismatch: '+ident)
    required_roles = {'ORCH','HST','STO','CAP','REC','MEM','HIE','VEC','HEL','PRV','PER','DST','REW','OPS','QA','AUD'}
    require(set(index['agents']) == required_roles, 'role inventory mismatch')
    common=range_set(index['common_ranges'],len(lines))
    for role, profile in index['agents'].items():
        covered = range_set(profile['full_ranges'],len(lines))
        require(common <= covered, 'missing common ranges: '+role)
        if role in ('ORCH','AUD'):
            require(len(covered)==len(lines) and set(profile['rule_ids'])==set(index['rules']), 'incomplete full read: '+role)
        else:
            for section in profile['additional_sections']:
                require(range_set([index['sections'][section]],len(lines))<=covered, 'missing section: '+role+' '+section)
            for ident, rule in index['rules'].items():
                if set(profile['task_tags']) & set(rule['tasks']) or ident in profile['rule_ids']:
                    require(ident in profile['rule_ids'] and range_set([rule['range']],len(lines))<=covered,'missing rule: '+role+' '+ident)
            require(set(profile['rule_ids'])<=set(index['rules']), 'unknown assigned rule: '+role)
    spec=(root/'reference/umca.md').read_bytes()
    require(hashlib.sha1(f'blob {len(spec)}\0'.encode()+spec).hexdigest()=='a6f7095bb1de8d8dac24302a3b4820232c844cb2','spec blob mismatch')
    spec_lines=spec.decode().splitlines()
    requirements={i:l[6:] for i,l in enumerate(spec_lines,1) if l.startswith('- [ ] ')}
    ledger=(root/'ACCEPTANCE_TRACEABILITY.md').read_text(encoding='utf-8')
    mapped={int(i):v.strip() for i,v in re.findall(r'^\| L(\d+) \| ([^|]+) \|',ledger,re.M)}
    require(len(requirements)==31 and mapped==requirements,'acceptance checkbox wording/coverage mismatch')
    graph=json.loads((root/'execution-graph.json').read_text(encoding='utf-8'))
    nodes=graph['nodes'];expanded=graph['expanded_static_dependencies']
    orch=(root/'00-orchestrator.md').read_text(encoding='utf-8')
    rows={i:(owner.strip(),[] if deps.strip()=='—' else [v.strip() for v in deps.split(',')],accept.strip()) for i,owner,deps,accept in re.findall(r'^\| ([A-Z][A-Z0-9]+) \| ([^|]+) \| ([^|]+) \| ([^|]+) \|$',orch,re.M)}
    require(set(rows)==set(nodes) and len(nodes)==37,'orchestrator graph inventory mismatch')
    expected={}
    for unit,n in nodes.items():
        require(rows[unit]==(n['owner'],n['depends_on'],n['acceptance']),'graph row mismatch: '+unit)
        deps=n['depends_on']; merge=n['merge_after']
        require(set(deps+merge)<=set(nodes),'unknown dependency: '+unit)
        if n['kind']=='gate':
            require(not merge,'unexpected gate merge constraint: '+unit)
            expected[unit]=deps
        else:
            expected['CAND.'+unit]=deps
            expected['AUD.'+unit+'.candidate']=['CAND.'+unit]
            expected['MERGE.'+unit]=['AUD.'+unit+'.candidate']+merge
            expected['AUD.'+unit+'.integrated']=['MERGE.'+unit]
            expected[unit]=['AUD.'+unit+'.integrated']
    require(expected==expanded and len(expanded)==157,'expanded graph mismatch')
    require(all(d in expanded for deps in expanded.values() for d in deps),'unknown expanded dependency')
    require(len(tuple(graphlib.TopologicalSorter(expanded).static_order()))==157,'cyclic graph')
    from acquire_references import validate as validate_refs
    refs=validate_refs(json.loads((root/'reference/repos.json').read_text(encoding='utf-8')))
    require(len(refs)==20 and sum(len(e['files']) for e in refs)==81,'reference inventory mismatch')
    anchors=ranges=0
    for e in refs:
        require(set(e['roles'])<=required_roles,'unknown reference role')
        for a in e['anchors']:
            anchors+=1; ranges+=len(a['ranges'])
            require(a['archive_alias']==e['id'] and a['commit']==e['commit'] and a['path'] in e['files'],'reference anchor identity mismatch')
            f=e['files'][a['path']]
            require(a['blob_sha1']==f['blob_sha1'] and a['lines']==f['lines'],'reference anchor file mismatch')
            range_set(a['ranges'],a['lines'])
    require((anchors,ranges)==(84,91),'reference anchor/range count mismatch')
    for role in required_roles-{'ORCH'}:
        candidates=[p for p in root.glob('[0-9][0-9]-*.md') if p.read_text(encoding='utf-8').startswith('# '+role+' ')]
        require(len(candidates)==1,'agent document mismatch: '+role)
        text=candidates[0].read_text(encoding='utf-8')
        require('GO_CONTEXT_READ_MAP.md' in text and 'UPSTREAM_PATCH_POLICY.md' in text,'missing mandatory policy: '+role)
    for p in root.rglob('*.md'):
        if p in (source,root/'reference/umca.md'): continue
        for link in re.findall(r'\]\(([^)]+)\)',p.read_text(encoding='utf-8')):
            if link.startswith(('https://','http://','#','mailto:')): continue
            require((p.parent/link.split('#',1)[0]).is_file(),f'broken link: {p.name}: {link}')
    if check_sums:
        sums={}
        for l in (root/'SHA256SUMS').read_text(encoding='utf-8').splitlines():
            h,p=l.split('  ',1)
            require(re.fullmatch('[a-f0-9]{64}',h) and p not in sums and not Path(p).is_absolute() and '..' not in Path(p).parts,'invalid checksum entry')
            sums[p]=h
        actual={p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file() and '__pycache__' not in p.parts and p.name!='SHA256SUMS'}
        require(actual==set(sums),'checksum inventory mismatch')
        for p,h in sums.items(): require(sha((root/p).read_bytes())==h,'checksum mismatch: '+p)
    return {'context_rules':248,'roles':16,'checklist_rows':31,'reference_repositories':20,'reference_files':81,'reference_anchors':84,'reference_ranges':91,'main_nodes':37,'expanded_nodes':157,'checksums_verified':check_sums,'status':'PASS (offline consistency only)'}


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--root',type=Path,default=Path(__file__).resolve().parent.parent)
    p.add_argument('--no-checksums',action='store_true',help='authoring only, never for release/bootstrap')
    a=p.parse_args()
    try: print(json.dumps(validate(a.root,not a.no_checksums),indent=2)); return 0
    except (OSError,ValueError,KeyError,TypeError,graphlib.CycleError) as e:
        print('FAIL: '+str(e),file=sys.stderr);return 1

if __name__=='__main__': sys.exit(main())
