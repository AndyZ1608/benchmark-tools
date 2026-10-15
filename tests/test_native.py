"""Optimized native pointer-chase correctness regression; requires a local C compiler.
Run on Linux: python3 tests/test_native.py. No timed stress workload is run here.
"""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1] / 'native' / 'compute-bench.c'

class NativeIntegrity(unittest.TestCase):
    def test_pointer_chase_cycle_and_corruption_are_observable(self):
        compiler = shutil.which(os.environ.get('CC', 'cc'))
        if compiler is None:
            self.skipTest('C compiler unavailable')
        with tempfile.TemporaryDirectory(prefix='vmbench native fixture ') as directory:
            root = pathlib.Path(directory)
            source = root / 'cycle.c'
            source.write_text('#define main vmbench_cli_main\n#include "' + SOURCE.as_posix() + '"\n#undef main\n' + r'''
int main(void) {
    size_t chain[128];
    for (size_t i = 0; i < 128; ++i) chain[i] = (i + 1) % 128;
    Job job = {.kind = 6, .n = 128, .chain = chain};
    mem_batch(&job);
    if (job.count != 1 || job.errors != 0) return 1;
    chain[127] = 127; /* Known broken cycle must not look like a valid traversal. */
    mem_batch(&job);
    if (job.count != 2 || job.errors != 1) return 2;
    return 0;
}
''')
            binary = root / 'cycle'
            subprocess.run([compiler, '-O2', '-fno-fast-math', '-pthread', str(source), '-lm', '-o', str(binary)], check=True, capture_output=True)
            subprocess.run([str(binary)], check=True, timeout=5, capture_output=True)

if __name__ == '__main__':
    unittest.main(verbosity=2)
