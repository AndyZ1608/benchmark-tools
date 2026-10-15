#!/usr/bin/env python3
"""Create a source-only development snapshot, optionally with partial local binaries.
Never claims to be a complete offline release. No downloads or system changes.
"""
import argparse,hashlib,io,pathlib,struct,tarfile
root=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--binaries-dir',type=pathlib.Path,help='Explicit directory containing vmbench and engines/compute-bench')
args=parser.parse_args();out=root/'dist';out.mkdir(exist_ok=True)
items=[]
for name in ['cmd','internal','native','scripts','tests','docs','config','LICENSES','third-party']:
 items.extend(p for p in (root/name).rglob('*') if p.is_file() and '__pycache__' not in p.parts)
items.extend(root/name for name in ['.gitignore','.gitattributes','go.mod','README.md','VERSION','LICENSE','BUILD_STATUS.md','init.sh','start.sh'])
label='source';binaries=[]
if args.binaries_dir:
 for name in ['vmbench','engines/compute-bench']:
  p=args.binaries_dir/name;b=p.read_bytes()
  if b[:6]!=b'\x7fELF\x02\x01':raise SystemExit(f'Not Linux ELF64: {p}')
  arch={62:'amd64',183:'arm64'}.get(struct.unpack_from('<H',b,18)[0])
  if arch is None or (label!='source' and label!=arch):raise SystemExit('Mixed/unsupported development binaries')
  label=arch;binaries.append((p,name))
archive=out/f'vmbench-development-{label}.tar.gz'
with tarfile.open(archive,'w:gz') as t:
 for p,name in [(p,p.relative_to(root).as_posix()) for p in sorted(items)]+binaries:
  data=p.read_bytes()
  if p.suffix in ['.sh','.go','.py','.md','.yaml','.json'] or p.name in ['go.mod','VERSION']:data=data.replace(b'\r\n',b'\n')
  info=t.gettarinfo(str(p),'vmbench/'+name);info.size=len(data)
  if p.suffix=='.sh' or name in ['vmbench','engines/compute-bench']:info.mode=0o755
  t.addfile(info,io.BytesIO(data))
sha=hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_suffix(archive.suffix+'.sha256').write_text(f'{sha}  {archive.name}\n')
print(f'{archive}; development snapshot only; not a complete offline binary release')
