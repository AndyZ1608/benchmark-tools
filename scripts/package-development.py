#!/usr/bin/env python3
"""Package a clearly labelled development snapshot, never a complete release."""
import pathlib,tarfile,hashlib
root=pathlib.Path(__file__).resolve().parent.parent
out=root/'dist';out.mkdir(exist_ok=True)
items=[]
for name in ['cmd','internal','native','scripts','tests','docs','config','LICENSES','third-party']:
    items.extend(p for p in (root/name).rglob('*') if p.is_file() and '__pycache__' not in p.parts)
items.extend(root/name for name in ['go.mod','README.md','VERSION','LICENSE','BUILD_STATUS.md','init.sh','start.sh'])
archive=out/'vmbench-development-linux-amd64.tar.gz'
with tarfile.open(archive,'w:gz') as t:
    for p in sorted(items):
        info=t.gettarinfo(str(p),'vmbench/'+p.relative_to(root).as_posix())
        if p.suffix=='.sh':info.mode=0o755
        with p.open('rb') as f:t.addfile(info,f)
    for source,target in [('dist/vmbench/vmbench','vmbench/vmbench'),('dist/vmbench/engines/compute-bench','vmbench/engines/compute-bench')]:
        p=root/source;info=t.gettarinfo(str(p),target);info.mode=0o755
        with p.open('rb') as f:t.addfile(info,f)
sha=hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_suffix(archive.suffix+'.sha256').write_text(f'{sha}  {archive.name}\n')
with tarfile.open(archive) as t:
    names=t.getnames()
    assert 'vmbench/vmbench' in names and 'vmbench/engines/compute-bench' in names
    assert 'vmbench/BUILD_STATUS.md' in names
    assert not any('/.tools/' in n for n in names)
print(f'{archive}: verified {len(names)} entries; sha256={sha}')
