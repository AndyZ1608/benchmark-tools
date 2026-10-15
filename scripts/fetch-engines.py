#!/usr/bin/env python3
"""Verify pinned engine sources and prepare an ignored build cache.

Checked-in third-party archives are read-only inputs. --offline never downloads.
"""
import argparse
import hashlib
import json
import pathlib
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--offline', action='store_true')
args = parser.parse_args()
target = ROOT / '.vmbench/cache/engine-sources'
for entry in json.loads((ROOT / 'scripts/engines.lock.json').read_text()):
    name = entry['name'] + '.tar.gz'
    cached = target / name
    shipped = ROOT / 'third-party' / name
    source = shipped if shipped.exists() else cached
    if source.exists():
        data = source.read_bytes()
    else:
        if args.offline:
            raise SystemExit(f'Missing offline source: {shipped} (or {cached})')
        with urllib.request.urlopen(entry['url'], timeout=60) as response:
            data = response.read()
    if hashlib.sha256(data).hexdigest() != entry['sha256']:
        raise SystemExit(f'Checksum mismatch: {source}')
    target.mkdir(parents=True, exist_ok=True)
    if not cached.exists() or cached.read_bytes() != data:
        cached.write_bytes(data)
    print(f"Verified {entry['name']}: {entry['sha256']}")
