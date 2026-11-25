#!/usr/bin/env python3
"""Read-only inherited-path marker and footprint check. Review semantics separately."""
import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path

MARK = re.compile(r'^\s*(//|--|#|<!--)\s+UMCA-(BEGIN|END):\s+([A-Za-z0-9][A-Za-z0-9_.-]*?)\.\s*(-->)?\s*$')
HUNK = re.compile(rb'^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@', re.M)
KINDS = {'generated', 'commentless', 'directive', 'binary', 'deletion', 'mode', 'formatting'}


def git(repo, *args):
    # Resolve the requested repository and committed objects, not ambient Git
    # worktree/index/object overrides or refs/replace from a developer session.
    env = {k: v for k, v in os.environ.items() if not k.startswith('GIT_')}
    env.update(GIT_NO_REPLACE_OBJECTS='1', GIT_CONFIG_NOSYSTEM='1',
               GIT_CONFIG_GLOBAL=os.devnull, GIT_OPTIONAL_LOCKS='0', LC_ALL='C')
    return subprocess.check_output(['git', '-C', str(repo), '-c',
                                    'core.attributesFile='+os.devnull, *args],
                                   env=env, stderr=subprocess.PIPE, timeout=120)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def tree(repo, rev):
    result = {}
    for entry in git(repo, 'ls-tree', '-rz', '--full-tree', rev).split(b'\0'):
        if not entry:
            continue
        meta, path = entry.split(b'\t', 1)
        mode, typ, oid = meta.decode('ascii').split(' ')
        result[path.decode('utf-8', 'surrogateescape')] = (mode, typ, oid)
    return result


def blob(repo, entry):
    if entry is None:
        return b''
    # Gitlinks are not regular blobs; the recorded commit is their content identity.
    if entry[1] != 'blob':
        return entry[2].encode('ascii')
    return git(repo, 'cat-file', 'blob', entry[2])


def parse_markers(data):
    regions = []
    errors = []
    active = None
    seen = set()
    for n, line in enumerate(data.decode('utf-8', 'surrogateescape').split('\n'), 1):
        match = MARK.fullmatch(line)
        if not match or (match[1] == '<!--') != bool(match[4]):
            # Only flag near-markers in standalone comment syntax, not mentions in strings.
            if re.match(r'^\s*(?://|--|#|<!--)\s+UMCA-(?:BEGIN|END):', line):
                errors.append(f'line {n}: malformed delimiter')
            continue
        style, kind, ident, _ = match.groups()
        if kind == 'BEGIN':
            if active is not None:
                errors.append(f'line {n}: nested marker {ident} within {active[0]}')
                continue
            if ident in seen:
                errors.append(f'line {n}: duplicate ID {ident}')
            seen.add(ident)
            active = (ident, n, style)
        elif active is None or active[0] != ident or active[2] != style:
            errors.append(f'line {n}: unmatched/crossed/mismatched-style END {ident}')
        else:
            regions.append({'id': ident, 'start': active[1], 'end': n})
            active = None
    if active is not None:
        errors.append(f'line {active[1]}: unclosed {active[0]}')
    return regions, errors


def run(args):
    repo = Path(args.repo).resolve()
    base = git(repo, 'rev-parse', '--verify', '--end-of-options', args.base + '^{commit}').decode().strip()
    head = git(repo, 'rev-parse', '--verify', '--end-of-options', args.head + '^{commit}').decode().strip()
    git(repo, 'merge-base', '--is-ancestor', base, head)
    old, new = tree(repo, base), tree(repo, head)
    inherited = sorted(p for p in old if old[p] != new.get(p))
    added = sorted(set(new) - set(old))
    # No working-tree inspection: only committed objects are audited.
    exceptions = json.loads(Path(args.exceptions).read_text(encoding='utf-8')) if args.exceptions else []
    if not isinstance(exceptions, list):
        raise ValueError('exceptions must be a JSON array')
    by_path = {}
    problems = []
    seen_ids = set()
    for e in exceptions:
        if not isinstance(e, dict) or not isinstance(e.get('path'), str):
            raise ValueError('exception must be an object with path')
        if e['path'] in by_path:
            raise ValueError('duplicate path in exception manifest: ' + e['path'])
        if not isinstance(e.get('id'), str) or not e['id'] or e['id'] in seen_ids:
            raise ValueError('missing/duplicate exception ID')
        seen_ids.add(e['id'])
        by_path[e['path']] = e
    result = {'base': base, 'head': head, 'git_version': git(repo, '--version').decode().strip(), 'new_paths': added, 'inherited_paths': [],
              'summary': {'modified_inherited_files': len(inherited), 'new_files': len(added),
                          'added_inherited_lines': 0, 'deleted_inherited_lines': 0,
                          'marker_blocks': 0, 'approved_exception_files': 0}, 'errors': problems}
    for path in inherited:
        o, n = old[path], new.get(path)
        before, after = blob(repo, o), blob(repo, n)
        # Explicit options plus literal pathspec protect paths beginning with glob magic.
        patch = git(repo, '--literal-pathspecs', 'diff', '--no-renames', '--no-ext-diff', '--no-textconv',
                    '--no-color', '--no-relative', '--ignore-submodules=none', '--src-prefix=a/', '--dst-prefix=b/', '--diff-algorithm=myers', '--no-indent-heuristic', '--unified=0', '--inter-hunk-context=0', '--word-diff=none',
                    '--output-indicator-new=+', '--output-indicator-old=-', '--output-indicator-context= ',
                    '--full-index', '--binary', base, head, '--', path)
        binary = b'\0' in before or b'\0' in after or b'GIT binary patch\n' in patch
        structural = []
        if n is None:
            structural.append('deleted-file')
        elif o[0:2] != n[0:2]:
            structural.append('mode-or-type-change')
        if binary:
            structural.append('binary-diff')
        if n is not None and n[1] != 'blob':
            structural.append('non-blob')
        if n is not None and n[0] == '120000':
            structural.append('symlink')
        regions, errors = parse_markers(after) if not structural else ([], [])
        uncovered = []
        additions = deletions = 0
        for h in HUNK.finditer(patch):
            a, ac, b, bc = (int(h[1]), int(h[2] or b'1'), int(h[3]), int(h[4] or b'1'))
            additions += bc
            deletions += ac
            # A replacement/addition is covered when every changed new line is inside
            # a marker region. A pure deletion has a gap after b: a surviving pair
            # must bracket that gap. Human review checks the intended old anchor.
            if bc:
                covered = all(any(r['start'] <= line <= r['end'] for r in regions)
                              for line in range(b, b + bc))
            else:
                covered = any(r['start'] <= b < r['end'] for r in regions)
            if not covered:
                uncovered.append({'old_start': a, 'old_count': ac, 'new_start': b, 'new_count': bc})
        # An unclassified nontext change must never slip through zero parsed hunks.
        if not additions and not deletions and before != after and not structural:
            structural.append('nontext-or-unparsed-change')
        evidence = {'path': path, 'base_sha256': digest(before), 'head_sha256': digest(after),
                    'patch_sha256': digest(patch), 'uncovered_hunks': uncovered, 'structural': structural}
        record = {**evidence, 'added_lines': additions, 'deleted_lines': deletions,
                  'markers': regions, 'marker_errors': errors, 'exception_status': 'not-needed'}
        result['summary']['added_inherited_lines'] += additions
        result['summary']['deleted_inherited_lines'] += deletions
        result['summary']['marker_blocks'] += len(regions)
        for err in errors:
            problems.append(f'{path}: {err}')
        approval = by_path.pop(path, None)
        if uncovered or structural:
            record['proposed_exception'] = evidence  # evidence only, not approval
            required = ('id', 'kind', 'reason', 'owner', 'reviewer', 'evidence')
            valid = (approval is not None and all(approval.get(k) for k in required)
                     and approval['kind'] in KINDS and approval['owner'] != approval['reviewer']
                     and all(approval.get(k) == v for k, v in evidence.items()))
            if valid:
                record['exception_status'] = 'hash-matched-review-record'
                record['exception_id'] = approval['id']
                result['summary']['approved_exception_files'] += 1
            else:
                record['exception_status'] = 'missing-or-stale'
                problems.append(f'{path}: uncovered hunk/structural change lacks exact approved exception')
        elif approval is not None:
            problems.append(f'{path}: stale/unnecessary exception must be removed')
        result['inherited_paths'].append(record)
    for path in by_path:
        problems.append(f'{path}: exception is not a modified inherited path')
    result['status'] = 'PASS' if not problems else 'FAIL'
    result['limits'] = 'Mechanical committed-tree check only; independent reviewer must verify syntax, scope, exact exception evidence, generator provenance, actual tests, and clean worktree.'
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default='.')
    parser.add_argument('--base', required=True)
    parser.add_argument('--head', default='HEAD')
    parser.add_argument('--exceptions', help='JSON array of individually approved exact exceptions')
    parser.add_argument('--output', help='JSON report destination (outside repo recommended)')
    args = parser.parse_args()
    try:
        report = run(args)
        encoded = json.dumps(report, indent=2, ensure_ascii=True) + '\n'
        if args.output:
            Path(args.output).write_text(encoded, encoding='utf-8')
            print(f"{report['status']}: {args.output} ({len(report['errors'])} error(s))")
        else:
            print(encoded, end='')
        return int(report['status'] != 'PASS')
    except (OSError, ValueError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as err:
        print(f'ERROR: {err}', file=sys.stderr)
        if isinstance(err, subprocess.CalledProcessError) and err.stderr:
            print(err.stderr.decode(errors='replace'), file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
