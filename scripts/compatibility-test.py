#!/usr/bin/env python3
"""Run bounded qualification phases and always write machine-readable evidence.

--install authorizes source init package installation inside a disposable CI image.
--workloads explicitly authorizes bounded CPU/RAM and isolated file-based disk tests.
"""
import argparse, datetime, json, os, pathlib, re, shutil, subprocess, tempfile

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--install',action='store_true')
parser.add_argument('--workloads',action='store_true')
parser.add_argument('--output',type=pathlib.Path,default=pathlib.Path('compatibility-result.json'))
args=parser.parse_args()
root=pathlib.Path(__file__).resolve().parents[1]
result={'schema':1,'timestamp':datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'os_release':pathlib.Path('/etc/os-release').read_text(), 'architecture':os.uname().machine,
        'environment':'container' if pathlib.Path('/.dockerenv').exists() or pathlib.Path('/run/.containerenv').exists() else 'host-or-vm',
        'host_vm_certification':False, 'phases':{}}
failed=False
def run(name,command,timeout=900,required_modules=()):
    global failed
    try:
        p=subprocess.run(command,cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=timeout,env=dict(os.environ,GOTOOLCHAIN='local',TEST_TELEMETRY_DIR=str(root/'scripts/go-telemetry')))
        missing=[module for module in required_modules if not re.search(r'^'+re.escape(module)+r' - (COMPLETED|UNCONFIRMED)$',p.stdout,re.M)]
        passed=p.returncode==0 and not missing
        result['phases'][name]={'status':'passed' if passed else 'failed','exit_code':p.returncode,'command':command,'output':p.stdout[-16000:]}
        if missing: result['phases'][name]['missing_workloads']=missing
        print(p.stdout,flush=True)
        failed|=not passed
        return passed
    except (OSError,subprocess.TimeoutExpired) as e:
        result['phases'][name]={'status':'failed','error':str(e)};failed=True;return False
try:
    run('shell_syntax',['bash','-c','find scripts -name "*.sh" -print0 | xargs -0 -n1 bash -n && bash -n init.sh && bash -n start.sh'])
    if shutil.which('shellcheck'):
        run('shellcheck',['shellcheck','-x','init.sh','start.sh','scripts/bootstrap-common.sh','scripts/install/platform.sh','scripts/install/packages.sh','scripts/build.sh','scripts/build-engines.sh','scripts/smoke-linux.sh'])
    else: result['phases']['shellcheck']={'status':'not-run','reason':'ShellCheck unavailable'}
    run('bootstrap_regressions',['python3','tests/test_bootstrap.py'])
    run('package_adapters',['python3','tests/test_platform.py'])
    run('native_correctness',['python3','tests/test_native.py'])
    init=['bash','init.sh','--yes'] if args.install else ['bash','init.sh','--offline']
    installed=run('installation',init)
    selected=shutil.which('go') or 'go'
    saved=root/'.vmbench/go-toolchain'
    if saved.exists():
        local=root/'.vmbench/toolchains'/saved.read_text().strip()/'bin/go'
        if local.is_file(): selected=str(local)
    run('unit_tests',[selected,'test','./...'])
    if installed:
        run('repeated_installation',['bash','init.sh','--offline'])
        run('read_only_check',['bash','init.sh','--check'])
        run('startup',['bash','start.sh','--offline','--help'])
        run('engine_runtime',[str(root/'vmbench'),'doctor'])
        for mode in ['quick','standard','max']: run(mode+'_parsing',[str(root/'vmbench'),mode,'--help'])
        if args.workloads:
            # File-based temporary directory only; no host device paths are accepted.
            with tempfile.TemporaryDirectory(prefix='vmbench-ci-disposable-') as scratch:
                sentinel=pathlib.Path(scratch)/'sentinel';sentinel.write_bytes(b'user data must survive\n')
                for mode in ['quick','standard','max']:
                    run(mode+'_bounded_workload',[str(root/'vmbench'),mode,'--non-interactive','--ack-max','--disk-path',scratch,'--max-workers','1','--max-ram','64MiB','--max-disk-data','16MiB','--max-disk-jobs','1','--max-disk-depth','2','--max-outstanding-io','2','--sample-seconds','1','--max-duration','10m','--max-cv','0.5'],required_modules=('CPU','MEMORY','DISK'))
                assert sentinel.read_bytes()==b'user data must survive\n'
                assert set(pathlib.Path(scratch).iterdir())=={sentinel}
            result['phases']['disk_isolation']={'status':'passed'}
        else: result['phases']['bounded_workloads']={'status':'not-run'}
    result['phases']['offline_release']={'status':'not-run','reason':'Requires separate complete release and network-disabled environment without development tools'}
finally:
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,indent=2)+'\n')
raise SystemExit(1 if failed else 0)
