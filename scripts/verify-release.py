#!/usr/bin/env python3
"""Read-only validation of a staged portable release (does not qualify other hosts)."""
import argparse,pathlib,subprocess
from release import inspect,ENGINES
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('root',type=pathlib.Path)
parser.add_argument('--arch',choices=['amd64','arm64'],required=True)
args=parser.parse_args();root=args.root.resolve()
for name in ENGINES:
    inspect(root/name,args.arch)
    subprocess.run([str(root/name),'--version'],check=True,timeout=10)
subprocess.run([str(root/'vmbench'),'doctor'],check=True,timeout=40)
print('Static ELF and safe runtime checks passed on this host. Other targets remain unqualified.')
