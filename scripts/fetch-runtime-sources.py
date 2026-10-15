#!/usr/bin/env python3
"""Build-machine only: verified runtime source/notice inputs for the recorded Alpine target.

These inputs match the qualified build's package revisions, not arbitrary host libraries.
No invocation from benchmarks/startup. --offline never performs a download.
"""
import argparse, hashlib, json, pathlib, tarfile, urllib.request

ROOT=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--offline',action='store_true')
args=parser.parse_args()
lock=json.loads((ROOT/'scripts/runtime-sources.lock.json').read_text())
target=ROOT/'third-party/runtime'
for entry in lock['files']:
    relative=pathlib.PurePosixPath(entry['path'])
    if relative.is_absolute() or '..' in relative.parts: raise SystemExit('Invalid runtime source path')
    path=target/pathlib.Path(relative)
    if not path.exists():
        if args.offline: raise SystemExit(f'Missing offline runtime source: {path}')
        path.parent.mkdir(parents=True,exist_ok=True)
        with urllib.request.urlopen(entry['url'],timeout=60) as response: data=response.read()
        if hashlib.sha256(data).hexdigest()!=entry['sha256']: raise SystemExit(f'Runtime source checksum mismatch: {path}')
        path.write_bytes(data)
    if hashlib.sha256(path.read_bytes()).hexdigest()!=entry['sha256']: raise SystemExit(f'Runtime source checksum mismatch: {path}')
    if path.name.endswith('.tar.gz'):
        with tarfile.open(path) as archive:
            for member in archive.getmembers():
                name=pathlib.PurePosixPath(member.name).name
                if member.isfile() and name in ['COPYING','COPYING.LIB','COPYRIGHT','LICENSE','zlib.h']:
                    notice=path.parent/'notices'/name;notice.parent.mkdir(exist_ok=True)
                    notice.write_bytes(archive.extractfile(member).read())
target.mkdir(parents=True,exist_ok=True)
(target/'SOURCE-INDEX.json').write_text(json.dumps(lock,indent=2)+'\n')
print('Runtime sources/notices verified for '+lock['build_target']+'; exact binary/linkage review remains required.')
