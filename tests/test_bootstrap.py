"""Bootstrap integration tests using controlled platform/compiler/engine fixtures.
These exercise real Bash startup/state workflows, not Linux benchmark execution.
Run: python tests/test_bootstrap.py (Git Bash works on Windows).
"""
import hashlib,json,os,pathlib,shutil,subprocess,tempfile,unittest,tarfile
REPO=pathlib.Path(__file__).resolve().parents[1]
BASH=os.environ.get('VMBENCH_TEST_BASH') or shutil.which('bash')
if os.name=='nt': BASH=os.environ.get('VMBENCH_TEST_BASH',r'C:\Program Files\Git\bin\bash.exe')
def shellpath(p):
    s=pathlib.Path(p).resolve().as_posix()
    return '/'+s[0].lower()+s[2:] if os.name=='nt' else s
class Bootstrap(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory(prefix='vmbench bootstrap spaces ')
        self.base=pathlib.Path(self.temp.name);self.root=self.base/'repository with spaces';self.root.mkdir()
        for name in ['init.sh','start.sh','go.mod','VERSION']:
            shutil.copy2(REPO/name,self.root/name)
        (self.root/'scripts').mkdir()
        shutil.copytree(REPO/'scripts/install',self.root/'scripts/install')
        shutil.copytree(REPO/'scripts/go-telemetry',self.root/'scripts/go-telemetry')
        for name in ['bootstrap-common.sh','go-toolchain.lock']:
            shutil.copy2(REPO/'scripts'/name,self.root/'scripts'/name)
        (self.root/'cmd/vmbench').mkdir(parents=True);(self.root/'cmd/vmbench/main.go').write_text('package main\n')
        (self.root/'internal').mkdir();(self.root/'native').mkdir();(self.root/'native/compute-bench.c').write_text('/* fixture */\n')
        self.bin=self.base/'bin';self.bin.mkdir();self.templates=self.base/'templates';self.templates.mkdir()
        self.env=dict(os.environ)
        self.env.pop('GOTOOLCHAIN',None)
        os_release=self.base/'os-release';os_release.write_text('ID=ubuntu\nVERSION_ID="24.04"\n')
        systempath='/usr/bin:/bin'
        if os.name != 'nt':
            # Missing-engine/compiler cases must not discover real host tools
            # installed beside the fixture. Keep only base shell utilities.
            systemtools=self.base/'base-tools';systemtools.mkdir()
            for name in ['bash','sh','env','timeout','cat','cp','mv','rm','mkdir','rmdir','chmod','touch','ln','ls','wc','id','dirname','basename','readlink','realpath','stat','sha256sum','awk','sed','grep','tr','cut','head','tail','tee','sort','find','xargs','mktemp','date','sleep','tar','gzip','readelf','file','du','getconf']:
                executable=shutil.which(name)
                if executable:(systemtools/name).symlink_to(executable)
            systempath=shellpath(systemtools)
        self.env.update(BOOTSTRAP_FIXTURE_PATH=shellpath(self.bin)+':'+systempath,PATH=shellpath(self.bin)+':'+systempath,VMBENCH_OS_RELEASE=shellpath(os_release),FIXTURE_TEMPLATES=shellpath(self.templates),FIXTURE_LOG=shellpath(self.base/'calls'),FIXTURE_ARGS=shellpath(self.base/'args'))
        self.write(self.bin/'uname','case "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n')
        self.write(self.bin/'od','case " $* " in *" -N4 "*) echo "7f 45 4c 46";; *) echo 62;; esac\n')
        self.write(self.bin/'ldd','if [[ ${FIXTURE_BAD_LIB:-0} == 1 ]]; then echo "libmissing.so => not found"; else echo "not a dynamic executable" >&2; exit 1; fi\n')
        for name in ['curl','wget','apt-get','sudo','git','debconf-set-selections']:
            self.write(self.bin/name,f'echo "network-or-install {name}" >> "$FIXTURE_LOG"; exit 93\n')
        self.install_go('go1.23.12')
        self.write(self.bin/'gcc',r'''echo native-build >> "$FIXTURE_LOG"
if [[ ${FIXTURE_BUILD_FAIL:-0} == 1 ]]; then exit 77; fi
while (($#)); do if [[ $1 == -o ]]; then cp "$FIXTURE_TEMPLATES/compute-bench" "$2"; chmod +x "$2"; break; fi; shift; done
''')
        for name,version in [('vmbench','VMBench 1.0.0-dev'),('compute-bench','compute-bench-v1'),('fio','fio-3.39'),('stress-ng','stress-ng 0.18.10'),('iperf3','iperf 3.17.1')]:
            content=f'''# bootstrap-test-executable
case "${{1:-}}" in
--version|-v) echo '{version}';;
--enghelp) [[ ${{FIXTURE_ENGHELP_FAIL:-0}} != 1 ]] || {{ echo 'engine query failed' >&2; exit 1; }}; echo "${{FIXTURE_FIO_ENGINES:-io_uring libaio}}";;
doctor) [[ ${{FIXTURE_DOCTOR_FAIL:-0}} != 1 ]] || {{ echo 'injected runtime failure' >&2; exit 1; }}; echo 'CPU engine: READY'; echo 'Memory engine: READY'; echo 'io_uring: UNAVAILABLE'; echo 'libaio: AVAILABLE_NOT_VALIDATED'; echo 'Disk validation deferred';;
--help|version) echo '{version}';;
*) printf '%s\\0' "$@" > "$FIXTURE_ARGS"; exit "${{FIXTURE_CLI_EXIT:-0}}";;
esac
'''
            self.write(self.templates/name,content)
            if name in ['fio','stress-ng','iperf3']:shutil.copy2(self.templates/name,self.bin/name)
        # Source script execute bits are set explicitly on Linux; git modes are set separately.
        for p in [self.root/'init.sh',self.root/'start.sh']:p.chmod(0o755)
    def tearDown(self):self.temp.cleanup()
    def write(self,p,body):p.write_text('#!/usr/bin/env bash\nset -e\n'+body,newline='\n');p.chmod(0o755)
    def install_go(self,version,location=None):
        p=location or self.bin/'go';p.parent.mkdir(parents=True,exist_ok=True)
        self.write(p,f'''[[ ${{GOTOOLCHAIN:-}} == local ]] || {{ echo unexpected-auto-toolchain >&2; exit 94; }}
[[ -r $TEST_TELEMETRY_DIR/mode ]] || {{ echo telemetry-policy-missing >&2; exit 98; }}
case "$1" in
version) echo 'go version {version} linux/amd64';;
env) echo /nonexistent-goenv;;
build) echo cli-build >> "$FIXTURE_LOG"; [[ ${{FIXTURE_CLI_BUILD_FAIL:-0}} != 1 ]] || exit 75; shift; while (($#)); do if [[ $1 == -o ]]; then cp "$FIXTURE_TEMPLATES/vmbench" "$2"; chmod +x "$2"; break; fi; shift; done;;
test) echo "go-test {version} ${{GOPROXY:-default}} cache:${{GOCACHE:-unset}} tmp:${{TMPDIR:-unset}} target:${{GOOS:-unset}}/${{GOARCH:-unset}} isa:${{GOAMD64:-unset}}/${{GOARM64:-unset}}" >> "$FIXTURE_LOG"
if [[ ${{FIXTURE_GO_TEST_FAIL:-0}} == 1 ]]; then echo 'FAIL: injected Go unit test failure' >&2; exit 1; fi;;
*) exit 95;; esac
''')
    def run_script(self,script='start.sh',args=(),cwd=None,ok=True):
        result=subprocess.run([BASH,'--noprofile','--norc','-c','export PATH="$BOOTSTRAP_FIXTURE_PATH"; exec bash "$@"','bootstrap-test',shellpath(self.root/script),*args],env=self.env,cwd=cwd or self.base,stdin=subprocess.DEVNULL,text=True,capture_output=True,timeout=90)
        if ok:self.assertEqual(result.returncode,0,result.stdout+result.stderr)
        else:self.assertNotEqual(result.returncode,0,result.stdout+result.stderr)
        return result
    def calls(self):
        p=self.base/'calls';return p.read_text() if p.exists() else ''
    def first(self):return self.run_script(args=['--offline','--help'])
    def prebuilts(self):
        (self.root/'engines').mkdir(exist_ok=True)
        for name in ['vmbench','compute-bench','fio','stress-ng','iperf3']:
            shutil.copy2(self.templates/name,self.root/name if name=='vmbench' else self.root/'engines'/name)
        # These fixture binaries represent known source builds. A real binary-only
        # release has no cmd/go.mod, while untracked binaries in source trees rebuild.
        state=self.root/'.vmbench';state.mkdir(exist_ok=True)
        script='set -euo pipefail; ROOT=$1; source "$ROOT/scripts/bootstrap-common.sh"; fingerprint cli > "$ROOT/.vmbench/cli-source.sha256"; fingerprint native > "$ROOT/.vmbench/native-source.sha256"'
        r=subprocess.run([BASH,'--noprofile','--norc','-c',script,'known-build-fixture',shellpath(self.root)],env=self.env,text=True,capture_output=True,timeout=30)
        self.assertEqual(r.returncode,0,r.stdout+r.stderr)
    def test_rhel_recovery_deferred_disk_reuses_dependencies(self):
        (self.base/'os-release').write_text('ID=rhel\nVERSION_ID="9.6"\n')
        self.prebuilts();self.env['FIXTURE_DOCTOR_FAIL']='1'
        self.run_script('init.sh',['--offline'],ok=False)
        self.assertFalse((self.root/'.vmbench/install-state.json').exists())
        before=self.calls();self.env.pop('FIXTURE_DOCTOR_FAIL')
        result=self.run_script('init.sh',['--offline'])
        self.assertIn('libaio: AVAILABLE_NOT_VALIDATED',result.stdout)
        self.assertIn('Disk validation deferred',result.stdout)
        self.assertEqual(self.calls(),before)
        self.assertNotIn('network-or-install',self.calls())
    def test_no_compiled_async_does_not_block_other_components(self):
        self.env['FIXTURE_FIO_ENGINES']='sync psync'
        self.first();before=self.calls()
        self.run_script(args=['cpu','quick','--non-interactive'])
        self.assertEqual(self.calls(),before)
    def test_fio_engine_query_failure_remains_fatal(self):
        self.env['FIXTURE_ENGHELP_FAIL']='1'
        self.run_script('init.sh',['--offline'],ok=False)
        self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_first_initialization(self):
        self.first();state=json.loads((self.root/'.vmbench/install-state.json').read_text());self.assertEqual(state['status'],'validated');self.assertIn('cli-build',self.calls());self.assertIn('native-build',self.calls());self.assertIn('go-test go1.23.12 off',self.calls());self.assertNotIn('network-or-install',self.calls())
    def test_second_launch_and_argument_forwarding(self):
        self.first();before=self.calls();self.run_script(args=['cpu','test','--path','a path with spaces','$(touch unsafe)']);self.assertEqual(self.calls(),before);self.assertEqual((self.base/'args').read_bytes().split(b'\0')[:-1],[b'cpu',b'test',b'--path',b'a path with spaces',b'$(touch unsafe)'])
    def test_component_modes_are_forwarded_without_rebuilding(self):
        self.first();before=self.calls()
        for component in ['cpu','ram','disk','network']:
            for mode in ['quick','standard','max']:
                args=[component,mode,'--non-interactive']
                if component=='disk':args+=['--path','/data path']
                if component=='network':args+=['--target','10.0.0.20']
                if mode=='max':args+=['--ack-max']
                self.run_script(args=args)
                self.assertEqual((self.base/'args').read_bytes().split(b'\0')[:-1],[a.encode() for a in args])
                self.assertEqual(self.calls(),before)
    def test_version_flags_work_through_startup_and_init_reuses(self):
        self.run_script(args=['--offline','--version']);before=self.calls()
        for argument in ['--version','-v','version']:
            result=self.run_script(args=[argument]);self.assertIn('VMBench 1.0.0-dev',result.stdout)
        self.run_script('init.sh',['--offline']);self.assertEqual(self.calls(),before)
    def test_legacy_cli_rebuilds_without_rebuilding_valid_engines(self):
        self.prebuilts();self.write(self.root/'vmbench','if [[ $1 == version ]]; then echo "VMBench 1.0.0-dev"; else echo "ERROR: expected component and action; use --help" >&2; exit 1; fi\n')
        self.run_script('init.sh',['--offline']);self.assertIn('cli-build',self.calls());self.assertNotIn('native-build',self.calls());self.assertNotIn('network-or-install',self.calls());before=self.calls();self.run_script('init.sh',['--offline']);self.assertEqual(self.calls(),before)
    def test_cli_version_failure_does_not_commit_state(self):
        self.write(self.templates/'vmbench','echo "ERROR: expected component and action; use --help" >&2; exit 1\n')
        result=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('vmbench --version failed',result.stderr);self.assertFalse((self.root/'.vmbench/install-state.json').exists());self.assertFalse((self.root/'.vmbench/install-state.sha256').exists())
    def test_version_source_change_rebuilds_only_cli(self):
        self.first();before=self.calls();(self.root/'VERSION').write_text('1.0.1-test\n');self.run_script(args=['--offline','--version']);delta=self.calls()[len(before):];self.assertIn('cli-build',delta);self.assertNotIn('native-build',delta)
    def test_go_122_offline_rejected(self):
        self.install_go('go1.22.2');r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('Offline initialization needs local Go',r.stderr);self.assertNotIn('network-or-install',self.calls());self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_go_122_uses_local_123(self):
        self.install_go('go1.22.2');self.install_go('go1.23.12',self.root/'.vmbench/toolchains/go1.23.12/bin/go');self.first();self.assertIn('go-test go1.23.12',self.calls())
    def test_newer_go_respected(self):
        self.install_go('go1.24.5');self.env['GOTOOLCHAIN']='go1.24.0+auto';self.first();self.assertIn('go-test go1.24.5',self.calls())
    def test_missing_gcc(self):
        (self.bin/'gcc').unlink();r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('build-essential',r.stderr);self.assertNotIn('network-or-install',self.calls())
    def test_missing_each_engine(self):
        for name in ['fio','stress-ng','iperf3']:
            with self.subTest(engine=name):
                p=self.bin/name;p.rename(self.bin/(name+'.saved'))
                try:
                    r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn(name,r.stderr);self.assertFalse((self.root/'.vmbench/install-state.json').exists())
                finally:(self.bin/(name+'.saved')).rename(p)
        self.assertNotIn('network-or-install',self.calls())
    def test_offline_complete_release_needs_no_compilers(self):
        self.prebuilts();(self.bin/'go').unlink();(self.bin/'gcc').unlink();self.first();self.assertNotIn('build',self.calls());self.assertNotIn('network-or-install',self.calls())
    def test_failed_initialization_no_success_state(self):
        self.env['FIXTURE_BUILD_FAIL']='1';self.run_script('init.sh',['--offline'],ok=False);self.assertFalse((self.root/'.vmbench/install-state.json').exists());self.assertFalse((self.root/'.vmbench/init.lock').exists());self.assertEqual(list((self.root/'.vmbench').glob('staging.*')),[])
    def test_cli_source_changes_only_cli_rebuilds(self):
        self.first();before=self.calls();(self.root/'cmd/vmbench/main.go').write_text('package main // changed\n');self.run_script(args=['--offline','--help']);delta=self.calls()[len(before):];self.assertIn('cli-build',delta);self.assertNotIn('native-build',delta)
    def test_presentation_source_change_rebuilds_only_cli(self):
        source=self.root/'internal/presentation/summary.go';source.parent.mkdir();source.write_text('package presentation\n')
        self.first();before=self.calls();source.write_text('package presentation // changed\n')
        self.run_script(args=['--offline','--help']);delta=self.calls()[len(before):]
        self.assertEqual(delta.count('cli-build'),1);self.assertNotIn('native-build',delta);self.assertNotIn('network-or-install',delta)
        before=self.calls();self.run_script(args=['--offline','--help']);self.assertEqual(self.calls(),before)
    def test_native_source_changes_only_native_rebuilds(self):
        self.first();before=self.calls();(self.root/'native/compute-bench.c').write_text('/* changed */\n');self.run_script(args=['--offline','--help']);delta=self.calls()[len(before):];self.assertNotIn('cli-build',delta);self.assertIn('native-build',delta)
    def test_automatic_controller_source_rebuilds_cli_and_forwards_mode(self):
        source=self.root/'internal/benchmark/auto/run.go';source.parent.mkdir(parents=True);source.write_text('package auto\n')
        self.first();before=self.calls();source.write_text('package auto // changed\n')
        self.run_script(args=['--offline','max','--ack-max','--disk-path','/data path'])
        delta=self.calls()[len(before):];self.assertEqual(delta.count('cli-build'),1);self.assertNotIn('native-build',delta);self.assertNotIn('network-or-install',delta)
        self.assertEqual((self.base/'args').read_bytes().split(b'\0')[:-1],[b'max',b'--ack-max',b'--disk-path',b'/data path'])
        before=self.calls();self.run_script(args=['--offline','quick']);self.assertEqual(self.calls(),before)
    def test_invalid_state_revalidated_without_build(self):
        self.first();before=self.calls();(self.root/'.vmbench/install-state.json').write_text('{broken');result=self.run_script(args=['--offline','--help']);self.assertIn('initializing VMBench',result.stdout);self.assertEqual(self.calls(),before);json.loads((self.root/'.vmbench/install-state.json').read_text())
    def test_state_checksum_enforced(self):
        self.first();p=self.root/'.vmbench/install-state.json';p.write_text(p.read_text().replace('"status": "validated"','"status": "corrupted"'));r=self.run_script(args=['--offline','--help']);self.assertIn('initializing VMBench',r.stdout);self.assertEqual(json.loads(p.read_text())['status'],'validated')
    def test_deleted_cli_rebuilds(self):
        self.first();(self.root/'vmbench').unlink();before=self.calls();self.run_script(args=['--offline','--help']);self.assertIn('cli-build',self.calls()[len(before):])
    def test_missing_library_never_launches(self):
        self.prebuilts();self.env['FIXTURE_BAD_LIB']='1';self.run_script('init.sh',['--offline'],ok=False);self.assertFalse((self.base/'args').exists());self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_rerun_is_idempotent_and_reinit_forces_build(self):
        self.first();before=self.calls();self.run_script('init.sh',['--offline']);self.assertEqual(self.calls(),before);self.run_script(args=['--offline','--reinit','--help']);delta=self.calls()[len(before):];self.assertIn('cli-build',delta);self.assertIn('native-build',delta)
    def test_exit_code_forwarding(self):
        self.first();self.env['FIXTURE_CLI_EXIT']='7';result=self.run_script(args=['version-other'],ok=False);self.assertEqual(result.returncode,7)
    def test_startup_flags_not_forwarded(self):
        self.run_script(args=['--offline','--yes','--','cpu','test']);self.assertEqual((self.base/'args').read_bytes(),b'cpu\0test\0')
    def test_online_yes_installs_only_missing_engine(self):
        (self.bin/'fio').unlink()
        self.write(self.bin/'sudo','exec "$@"\n')
        self.write(self.bin/'debconf-set-selections','cat >/dev/null\n')
        self.write(self.bin/'apt-get','echo "apt $*" >> "$FIXTURE_LOG"; if [[ $1 == install ]]; then cp "$FIXTURE_TEMPLATES/fio" "$(dirname "$0")/fio"; fi\n')
        self.run_script('init.sh',['--yes']);self.assertIn('apt update',self.calls());self.assertIn('apt install -y --no-install-recommends fio',self.calls());self.assertNotIn('upgrade',self.calls())
    def test_online_old_go_downloads_verified_local_toolchain(self):
        payload=self.base/'payload/go/bin/go';self.install_go('go1.23.12',payload)
        archive=self.base/'go.tar.gz'
        with tarfile.open(archive,'w:gz') as t:
            info=t.gettarinfo(str(payload),'go/bin/go');info.mode=0o755
            with payload.open('rb') as f:t.addfile(info,f)
        (self.root/'scripts/go-toolchain.lock').write_text(hashlib.sha256(archive.read_bytes()).hexdigest()+'  go1.23.12.linux-amd64.tar.gz\n')
        self.env['FIXTURE_GO_ARCHIVE']=shellpath(archive)
        self.install_go('go1.22.2')
        self.write(self.bin/'curl','echo go-download >> "$FIXTURE_LOG"; while (($#)); do if [[ $1 == -o ]]; then cp "$FIXTURE_GO_ARCHIVE" "$2"; break; fi; shift; done\n')
        self.run_script('init.sh');self.assertIn('go-download',self.calls());self.assertTrue((self.root/'.vmbench/toolchains/go1.23.12/bin/go').exists());self.assertIn('go-test go1.23.12',self.calls())
    def test_offline_bundled_prebuilt_release(self):
        self.prebuilts();(self.bin/'go').unlink();(self.bin/'gcc').unlink();self.run_script('init.sh',['--offline','--engine-mode','bundled']);self.assertNotIn('build',self.calls());self.assertNotIn('network-or-install',self.calls());self.assertEqual(json.loads((self.root/'.vmbench/install-state.json').read_text())['engine_mode'],'bundled')
    def test_wrong_engine_identity_rejected(self):
        self.prebuilts();self.write(self.root/'engines/fio','echo "Fiona 1.9"\n');self.write(self.bin/'fio','echo "Fiona 1.9"\n');r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('incompatible version',r.stderr);self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_online_packages_require_confirmation(self):
        (self.bin/'fio').unlink();r=self.run_script('init.sh',ok=False);self.assertIn('needs confirmation',r.stderr);self.assertNotIn('network-or-install',self.calls())
    def test_termination_during_build_leaves_no_state(self):
        self.write(self.bin/'gcc','kill -TERM "$PPID"; sleep 0.1; exit 0\n');r=self.run_script('init.sh',['--offline'],ok=False);self.assertEqual(r.returncode,143);self.assertFalse((self.root/'.vmbench/install-state.json').exists());self.assertFalse((self.root/'.vmbench/init.lock').exists())
    def test_offline_rejects_go_download_even_with_auto(self):
        self.install_go('go1.22.2');self.env['GOTOOLCHAIN']='go1.23.12+auto';self.run_script('init.sh',['--offline'],ok=False);self.assertNotIn('network-or-install',self.calls())
    def test_gotoolchain_path_never_downloads_online(self):
        self.install_go('go1.22.2');self.env['GOTOOLCHAIN']='path';r=self.run_script('init.sh',ok=False);self.assertIn('prohibits downloads',r.stderr);self.assertNotIn('network-or-install',self.calls())
    def test_stale_lock_fails_clearly(self):
        (self.root/'.vmbench/init.lock').mkdir(parents=True);r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('lock exists',r.stderr)
    def test_unsupported_os_and_arch(self):
        self.write(self.bin/'uname','case "$1" in -s) echo Linux;; -m) echo riscv64;; esac\n');r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('Unsupported architecture',r.stderr)
    def test_check_is_read_only_without_installation(self):
        result=self.run_script('init.sh',['--check'],ok=False)
        self.assertIn('Operating System',result.stdout);self.assertFalse((self.root/'.vmbench').exists());self.assertEqual(self.calls(),'')
    def test_check_existing_binaries_is_read_only(self):
        self.prebuilts();before={p.relative_to(self.root):p.read_bytes() for p in self.root.rglob('*') if p.is_file()}
        self.run_script('init.sh',['--check']);after={p.relative_to(self.root):p.read_bytes() for p in self.root.rglob('*') if p.is_file()}
        self.assertEqual(before,after);self.assertEqual(self.calls(),'')
    def test_unknown_os_never_installs(self):
        Path=pathlib.Path;Path(self.env['VMBENCH_OS_RELEASE'] if os.name!='nt' else self.base/'os-release').write_text('ID=unknown\nID_LIKE=debian\n')
        r=self.run_script('init.sh',['--yes'],ok=False);self.assertIn('Unsupported OS identity',r.stderr);self.assertNotIn('network-or-install',self.calls());self.assertFalse((self.root/'.vmbench').exists())
    def test_vendor_bundle_has_priority_and_records_version(self):
        (self.root/'vendor-engines').mkdir();shutil.copy2(self.templates/'fio',self.root/'vendor-engines/fio');self.first()
        state=json.loads((self.root/'.vmbench/install-state.json').read_text());self.assertEqual(state['engines']['fio']['origin'],'vendor-bundle');self.assertIn('3.39',state['engines']['fio']['version'])
    def test_schema_migration_revalidates_without_rebuilding(self):
        self.first();p=self.root/'.vmbench/install-state.json';p.write_text(p.read_text().replace('"schema": 2','"schema": 1'));before=self.calls()
        self.run_script(args=['--offline','--help']);self.assertEqual(self.calls(),before);self.assertEqual(json.loads(p.read_text())['schema'],2)
    def test_all_family_adapters_initialize_offline(self):
        for osid in ['debian','rocky','almalinux','rhel','centos','fedora','opensuse-leap','opensuse-tumbleweed','sles','arch','alpine']:
            with self.subTest(distribution=osid):
                (self.base/'os-release').write_text('ID='+osid+'\nVERSION_ID="test"\n')
                self.run_script('init.sh',['--offline']);self.assertNotIn('network-or-install',self.calls())
    def test_arm64_checks_elf_architecture(self):
        self.write(self.bin/'uname','case "$1" in -s) echo Linux;; -m) echo aarch64;; esac\n')
        self.prebuilts();r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('arm64 ELF',r.stderr)
    def test_explicit_old_go_is_not_overridden(self):
        self.env['GOTOOLCHAIN']='go1.22.2';r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('Explicit GOTOOLCHAIN',r.stderr);self.assertNotIn('network-or-install',self.calls())
    def test_go_cache_and_temporary_paths_stay_project_local(self):
        self.first();calls=self.calls();self.assertIn('cache:'+shellpath(self.root)+'/.vmbench/cache/go-build',calls);self.assertIn('tmp:'+shellpath(self.root)+'/.vmbench/staging.',calls)
    def test_invalid_go_telemetry_policy_blocks_tool_execution(self):
        (self.root/'scripts/go-telemetry/mode').write_text('on\n')
        r=self.run_script('init.sh',['--offline'],ok=False);self.assertIn('telemetry-off policy',r.stderr);self.assertNotIn('go-test',self.calls());self.assertNotIn('network-or-install',self.calls());self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_untracked_source_checkout_binaries_are_rebuilt(self):
        self.prebuilts()
        for name in ['cli-source.sha256','native-source.sha256']:(self.root/'.vmbench'/name).unlink()
        self.run_script('init.sh',['--offline']);self.assertIn('cli-build',self.calls());self.assertIn('native-build',self.calls())
    def test_build_recipe_change_rebuilds_source_components(self):
        self.first();before=self.calls();p=self.root/'init.sh';p.write_text(p.read_text()+'\n# changed build recipe\n',newline='\n')
        self.run_script(args=['--offline','--help']);delta=self.calls()[len(before):];self.assertIn('cli-build',delta);self.assertIn('native-build',delta)
    def test_verbose_build_failure_is_not_hidden_by_tee(self):
        self.env['FIXTURE_CLI_BUILD_FAIL']='1';r=self.run_script('init.sh',['--offline','--verbose'],ok=False);self.assertIn('CLI compilation failed',r.stdout+r.stderr);self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_verbose_native_failure_does_not_commit_state(self):
        self.env['FIXTURE_BUILD_FAIL']='1';r=self.run_script('init.sh',['--offline','--verbose'],ok=False);self.assertIn('Native compilation failed',r.stdout+r.stderr);self.assertFalse((self.root/'.vmbench/install-state.json').exists())
    def test_cross_compile_environment_does_not_leak_into_native_install(self):
        self.env.update(GOOS='windows',GOARCH='arm64',GOAMD64='v3');self.first();self.assertIn('target:linux/amd64 isa:v1/v8.0',self.calls())
    def test_unit_test_failure_never_publishes_staged_builds(self):
        self.env['FIXTURE_GO_TEST_FAIL']='1'
        for verbose in [False,True]:
            with self.subTest(verbose=verbose):
                args=['--offline']+(['--verbose'] if verbose else [])
                r=self.run_script('init.sh',args,ok=False);output=r.stdout+r.stderr
                self.assertIn('CLI build completed',output);self.assertIn('Go unit tests failed',output);self.assertIn('failed during Go unit tests',output)
                self.assertFalse((self.root/'vmbench').exists());self.assertFalse((self.root/'engines/compute-bench').exists())
                self.assertFalse((self.root/'.vmbench/install-state.json').exists());self.assertFalse((self.root/'.vmbench/install-state.sha256').exists())
                self.assertFalse((self.root/'.vmbench/init.lock').exists());self.assertEqual(list((self.root/'.vmbench').glob('staging.*')),[])
                self.assertIn('FAIL: injected Go unit test failure',(self.root/'.vmbench/last-build.log').read_text())
    def test_failed_unit_tests_preserve_prior_validated_installation(self):
        self.first()
        names=['vmbench','engines/compute-bench','engines/fio','engines/stress-ng','engines/iperf3','.vmbench/install-state.json','.vmbench/install-state.sha256','.vmbench/install-files.sha256','.vmbench/source.sha256','.vmbench/cli-source.sha256','.vmbench/native-source.sha256']
        before={name:(self.root/name).read_bytes() for name in names}
        self.env['FIXTURE_GO_TEST_FAIL']='1';r=self.run_script('init.sh',['--offline','--rebuild'],ok=False)
        self.assertIn('Go unit tests failed',r.stdout+r.stderr);self.assertEqual(before,{name:(self.root/name).read_bytes() for name in names})
        self.assertEqual(list((self.root/'.vmbench').glob('staging.*')),[])
if __name__=='__main__':unittest.main(verbosity=2)
