#!/usr/bin/env python3
"""Build-machine only: verified runtime source/notice inputs for the recorded Alpine target.

These inputs match the qualified build's package revisions, not arbitrary host libraries.
No invocation from benchmarks/startup. --offline never performs a download.
Tracked archives are read-only inputs; derived files go into the ignored cache.
"""
import argparse, hashlib, json, pathlib, tarfile, urllib.request

ROOT=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--offline',action='store_true')
args=parser.parse_args()
lock=json.loads((ROOT/'scripts/runtime-sources.lock.json').read_text())
shipped=ROOT/'third-party/runtime'
target=ROOT/'.vmbench/cache/runtime-sources'
for entry in lock['files']:
    relative=pathlib.PurePosixPath(entry['path'])
    if relative.is_absolute() or '..' in relative.parts: raise SystemExit('Invalid runtime source path')
    path=target/pathlib.Path(relative)
    source=shipped/pathlib.Path(relative)
    if not source.exists(): source=path
    if not source.exists():
        if args.offline: raise SystemExit(f'Missing offline runtime source: {path}')
        with urllib.request.urlopen(entry['url'],timeout=60) as response: data=response.read()
    else: data=source.read_bytes()
    if hashlib.sha256(data).hexdigest()!=entry['sha256']: raise SystemExit(f'Runtime source checksum mismatch: {source}')
    path.parent.mkdir(parents=True,exist_ok=True)
    if not path.exists() or path.read_bytes()!=data: path.write_bytes(data)
    if path.name.endswith('.tar.gz'):
        with tarfile.open(path) as archive:
            for member in archive.getmembers():
                name=pathlib.PurePosixPath(member.name).name
                if member.isfile() and name in ['COPYING','COPYING.LIB','COPYRIGHT','LICENSE','zlib.h']:
                    notice=path.parent/'notices'/name;notice.parent.mkdir(exist_ok=True)
                    notice.write_bytes(archive.extractfile(member).read())
target.mkdir(parents=True,exist_ok=True)
(target/'SOURCE-INDEX.json').write_bytes((json.dumps(lock,indent=2)+'\n').encode('utf-8'))
print('Runtime sources/notices verified for '+lock['build_target']+'; exact binary/linkage review remains required.')
