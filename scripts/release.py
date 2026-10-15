#!/usr/bin/env python3
"""Inspect static, architecture-specific bundles; record hashes, provenance and legal gate.

No downloads or package installation. Candidate archives are for local qualification only.
"""
import argparse, hashlib, json, pathlib, re, shutil, struct, subprocess, tarfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
ENGINES = ['vmbench', 'engines/compute-bench', 'engines/fio', 'engines/stress-ng', 'engines/iperf3']

def verify_audit_binaries(data, records):
    if data.get('binary_sha256') != {name: record['sha256'] for name, record in records.items()}:
        raise ValueError('Audit must record SHA-256 for exactly the five staged binaries; review changed builds again')

def inspect(path, arch):
    b = path.read_bytes()
    if len(b) < 64 or b[:6] != b'\x7fELF\x02\x01':
        raise ValueError(f'{path}: expected 64-bit little-endian ELF')
    if struct.unpack_from('<H', b, 18)[0] != {'amd64': 62, 'arm64': 183}[arch]:
        raise ValueError(f'{path}: wrong ELF architecture for {arch}')
    headers = subprocess.check_output(['readelf', '-l', str(path)], text=True)
    dynamic = subprocess.check_output(['readelf', '-d', str(path)], text=True)
    if 'INTERP' in headers or '(NEEDED)' in dynamic:
        raise ValueError(f'{path}: portable release requires static executables; shared libraries need a separate audited target')

def package(root, arch, libc, audit=None, candidate=False):
    root = root.resolve()
    target = f'linux-{arch}-{libc}-static'
    if not candidate and audit is None:
        raise ValueError('Release requires --audit FILE, covering libc/compiler runtime/libaio and every linked optional library. Use --candidate only for local qualification.')
    if audit is not None:
        data = json.loads(audit.read_text())
        if data.get('target') != target or data.get('approved') is not True:
            raise ValueError('Redistribution audit must approve the exact target')
        required = {'fio', 'stress-ng', 'iperf3', 'libc', 'compiler-runtime', 'libaio', 'go-runtime'}
        if not required <= set(data.get('components', {})):
            raise ValueError('Audit is missing transitive library/license components')
        for name, info in data['components'].items():
            if not re.fullmatch(r'[a-zA-Z0-9._-]+',name): raise ValueError('Invalid audit component name')
            if not info.get('license') or not info.get('source') or not info.get('reviewer'):
                raise ValueError(f'Audit incomplete for {name}: require license, source and reviewer')
            artifacts=info.get('artifacts',[])
            if not artifacts: raise ValueError(f'Missing bundled license/source artifacts for {name}')
            for relative in artifacts:
                source=(audit.parent/relative).resolve()
                if pathlib.Path(relative).is_absolute() or '..' in pathlib.Path(relative).parts or not source.is_file() or not source.is_relative_to(audit.parent.resolve()):
                    raise ValueError(f'Invalid audit artifact for {name}: {relative}')
                destination=root/'LICENSES'/'additional'/name/relative
                destination.parent.mkdir(parents=True,exist_ok=True)
                shutil.copy2(source,destination)
        shutil.copy2(audit, root / 'LICENSES' / 'redistribution-audit.json')
    records = {}
    for name in ENGINES:
        p = root / name
        inspect(p, arch)
        p.chmod(0o755)
        result = subprocess.run([str(p), '--version'], check=True, capture_output=True, text=True, timeout=10)
        records[name] = {'sha256': hashlib.sha256(p.read_bytes()).hexdigest(), 'version': result.stdout.strip()}
    if audit is not None:
        verify_audit_binaries(data, records)
    stale=root/'release-manifest.json'
    if stale.exists(): stale.unlink()
    subprocess.run([str(root/'vmbench'), 'doctor'], check=True, timeout=40)
    for name in ['fio', 'stress-ng', 'iperf3']:
        if not any((root/'LICENSES'/name).glob('*')):
            raise ValueError(f'Missing upstream licenses for {name}')
        archive = root/'LICENSES'/(name+'.tar.gz')
        pin = next(e for e in json.loads((ROOT/'scripts/engines.lock.json').read_text()) if e['name']==name)
        if not archive.is_file() or hashlib.sha256(archive.read_bytes()).hexdigest()!=pin['sha256']:
            raise ValueError(f'Missing or unverified corresponding upstream source for {name}')
    if not (root/'LICENSES/Go/LICENSE').is_file(): raise ValueError('Missing Go runtime notice')
    provenance = root/'LICENSES'/'engine-build.json'
    if not provenance.is_file(): raise ValueError('Missing engine-build.json provenance from build-engines.sh')
    built=json.loads(provenance.read_text())
    if built['arch']!=arch or built['libc']!=libc: raise ValueError('Engine build provenance target mismatch')
    for name in ['fio','stress-ng','iperf3']:
        if built['sha256'][name]!=records['engines/'+name]['sha256']: raise ValueError(f'Engine provenance hash mismatch: {name}')
    manifest = {'schema': 1, 'target': target, 'qualification': 'candidate-unqualified' if candidate else 'redistribution-reviewed-runtime-smoke-only', 'files': records,
                'network_required': False, 'host_vm_qualification': 'See docs/compatibility.json; build-host invocation does not certify other distributions'}
    (root/'release-manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
    suffix = '-candidate' if candidate else ''
    output = root.parent/(f'vmbench-{target}{suffix}.tar.gz')
    with tarfile.open(output, 'w:gz') as archive:
        archive.add(root, arcname='vmbench')
    checksum = hashlib.sha256(output.read_bytes()).hexdigest()
    output.with_suffix(output.suffix+'.sha256').write_text(f'{checksum}  {output.name}\n')
    print(output)
    return output

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root',type=pathlib.Path)
    parser.add_argument('--arch',choices=['amd64','arm64'],required=True)
    parser.add_argument('--libc',choices=['glibc','musl'],required=True)
    parser.add_argument('--audit',type=pathlib.Path)
    parser.add_argument('--candidate',action='store_true')
    args=parser.parse_args()
    try: package(args.root,args.arch,args.libc,args.audit,args.candidate)
    except (ValueError,OSError,subprocess.SubprocessError) as error: raise SystemExit(str(error))
