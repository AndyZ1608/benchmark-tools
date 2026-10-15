"""Fast adapter/detection regression tests with no package installation."""
import importlib.util,json,struct,os,pathlib,shutil,subprocess,tempfile,unittest
ROOT=pathlib.Path(__file__).resolve().parents[1]
BASH=os.environ.get('VMBENCH_TEST_BASH') or (r'C:\Program Files\Git\bin\bash.exe' if os.name=='nt' else shutil.which('bash'))
def path(p):
 s=pathlib.Path(p).resolve().as_posix();return '/'+s[0].lower()+s[2:] if os.name=='nt' else s
class Platform(unittest.TestCase):
 def shell(self,body,release='ID=debian\nVERSION_ID="13"\n'):
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d)/'os-release';p.write_text(release)
   script='set -Eeuo pipefail; ROOT='+"'"+path(ROOT)+"'"+'; source "$ROOT/scripts/bootstrap-common.sh"; uname(){ case "$1" in -s) echo Linux;; -m) echo x86_64;; -r) echo fixture-kernel;; esac; }; detect_linux; '+body
   return subprocess.run([BASH,'--noprofile','--norc','-c',script],env=dict(os.environ,VMBENCH_OS_RELEASE=path(p)),text=True,capture_output=True,timeout=25)
 def test_readme_and_script_permissions(self):
  text=(ROOT/'README.md').read_text()
  for snippet in ['chmod +x init.sh start.sh','./init.sh','./init.sh --rebuild','bash start.sh']:
   self.assertIn(snippet,text)
  for name in ['init.sh','start.sh']:
   if os.name!='nt':self.assertEqual((ROOT/name).stat().st_mode & 0o111,0o111)
 def test_git_script_modes(self):
  if not (ROOT/'.git').exists() or not shutil.which('git'):self.skipTest('requires Git checkout and Git executable')
  result=subprocess.run(['git','ls-files','--stage','init.sh','start.sh'],cwd=ROOT,text=True,capture_output=True,check=True)
  self.assertEqual(len(result.stdout.splitlines()),2)
  for line in result.stdout.splitlines():self.assertTrue(line.startswith('100755 '),line)
 def test_family_mapping(self):
  for identity,pm in [('ubuntu','apt-get'),('debian','apt-get'),('rocky','dnf'),('almalinux','dnf'),('rhel','dnf'),('centos','dnf'),('fedora','dnf'),('opensuse-leap','zypper'),('opensuse-tumbleweed','zypper'),('sles','zypper'),('arch','pacman'),('alpine','apk')]:
   with self.subTest(identity=identity):
    r=self.shell('echo "$PACKAGE_MANAGER"','ID='+identity+'\n');self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(r.stdout.strip(),pm)
 def test_unknown_like_is_not_install_authorization(self):
  r=self.shell('require_install_family','ID=unverified\nID_LIKE="debian ubuntu"\n');self.assertNotEqual(r.returncode,0);self.assertIn('Unsupported OS identity',r.stderr)
 def test_missing_metadata_is_clear(self):
  r=self.shell('platform_diagnostics; require_install_family','');self.assertNotEqual(r.returncode,0);self.assertIn('unknown',r.stdout)
 def test_metadata_is_not_evaluated(self):
  r=self.shell('echo "$OS_PRETTY"','ID=debian\nPRETTY_NAME="$(exit 77)"\n');self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(r.stdout.strip(),'$(exit 77)')
 def test_logical_package_translation(self):
  for family,compiler,aio in [('debian','build-essential','libaio-dev'),('rhel','gcc','libaio-devel'),('suse','gcc','libaio-devel'),('arch','gcc','libaio'),('alpine','build-base','libaio-dev')]:
   r=self.shell('FAMILY='+family+'; package_for compiler; package_for libaio');self.assertEqual(r.stdout.splitlines(),[compiler,aio])
 def test_package_command_adapters(self):
  for manager,expected in [('dnf','install -y --setopt=install_weak_deps=False fio'),('zypper','--non-interactive install --no-recommends fio'),('pacman','-S --needed --noconfirm fio'),('apk','add --no-cache fio')]:
   body=manager+'(){ printf "%s\\n" "$*"; }; PACKAGE_MANAGER='+manager+'; PRIVILEGE=sudo; sudo(){ "$@"; }; PACKAGES=(fio); YES=1 OFFLINE=0; install_packages'
   r=self.shell(body);self.assertEqual(r.returncode,0,r.stderr);self.assertIn(expected,r.stdout);self.assertNotIn('upgrade',r.stdout)
 def test_offline_blocks_package_managers(self):
  r=self.shell('PACKAGES=(fio); YES=1 OFFLINE=1; apt-get(){ echo FORBIDDEN; }; install_packages');self.assertNotEqual(r.returncode,0);self.assertNotIn('FORBIDDEN',r.stdout)
 def test_musl_static_fallback_uses_program_headers(self):
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d)/'engine';b=bytearray(120);b[:6]=b'\x7fELF\x02\x01';struct.pack_into('<Q',b,32,64);struct.pack_into('<H',b,54,56);struct.pack_into('<H',b,56,1);struct.pack_into('<I',b,64,1);p.write_bytes(b)
   r=self.shell("elf_no_interpreter '"+path(p)+"'");self.assertEqual(r.returncode,0,r.stderr)
   struct.pack_into('<I',b,64,3);p.write_bytes(b);r=self.shell("elf_no_interpreter '"+path(p)+"'");self.assertNotEqual(r.returncode,0)
 def test_wrong_compiler_architecture_is_rejected(self):
  r=self.shell("fakecc(){ echo '#define __aarch64__ 1'; }; check_compiler_target fakecc");self.assertNotEqual(r.returncode,0);self.assertIn('does not target x86_64',r.stderr)
 def test_mixed_compiler_libc_is_rejected(self):
  r=self.shell("fakecc(){ printf '#define __x86_64__ 1\\n#define __GLIBC__ 2\\n'; }; LIBC=musl; check_compiler_target fakecc");self.assertNotEqual(r.returncode,0);self.assertIn('libc headers',r.stderr)
 def test_release_gate_requires_audit(self):
  spec=importlib.util.spec_from_file_location('release',ROOT/'scripts/release.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  with tempfile.TemporaryDirectory() as d:
   with self.assertRaisesRegex(ValueError,'requires --audit'):m.package(pathlib.Path(d),'amd64','musl')
 def test_release_rejects_foreign_elf(self):
  spec=importlib.util.spec_from_file_location('release',ROOT/'scripts/release.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d)/'engine';b=bytearray(64);b[:6]=b'\x7fELF\x02\x01';b[18]=183;p.write_bytes(b)
   with self.assertRaisesRegex(ValueError,'wrong ELF architecture'):m.inspect(p,'amd64')
 def test_audit_cannot_be_reused_for_changed_binaries(self):
  spec=importlib.util.spec_from_file_location('release',ROOT/'scripts/release.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  records={name:{'sha256':'a'*64} for name in m.ENGINES}
  audit={'binary_sha256':{name:'a'*64 for name in m.ENGINES}}
  m.verify_audit_binaries(audit,records)
  records['engines/fio']['sha256']='b'*64
  with self.assertRaisesRegex(ValueError,'changed builds'):m.verify_audit_binaries(audit,records)
if __name__=='__main__':unittest.main(verbosity=2)
