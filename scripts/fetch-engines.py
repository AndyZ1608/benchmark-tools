#!/usr/bin/env python3
"""Build-machine only: fetch and verify source archives. Never invoked by VMBench."""
import pathlib,json,hashlib,urllib.request,sys
root=pathlib.Path(__file__).resolve().parent.parent
target=root/'third-party';target.mkdir(exist_ok=True)
for entry in json.loads((root/'scripts/engines.lock.json').read_text()):
    p=target/(entry['name']+'.tar.gz')
    if not p.exists():
        if '--offline' in sys.argv:raise SystemExit(f'Missing offline source: {p}')
        with urllib.request.urlopen(entry['url'],timeout=60) as response:p.write_bytes(response.read())
    if hashlib.sha256(p.read_bytes()).hexdigest()!=entry['sha256']:raise SystemExit(f'Checksum mismatch: {p}')
    print(f"Verified {entry['name']}: {entry['sha256']}")
