import os
import json
import pathlib
import subprocess
import tempfile
import unittest


SCRIPTS = pathlib.Path(__file__).resolve().parent


class LinuxPackagingTest(unittest.TestCase):
    def test_cuda_toolkit_reaches_native_submake(self):
        result = subprocess.run(
            ['make', '-n', '-C', str(SCRIPTS.parent), 'build-llamacpp-runtime',
             'MODELD_CUDA_TOOLKIT=12.9'], capture_output=True, text=True, check=True)
        self.assertIn('runtime MODELD_CUDA_TOOLKIT="12.9"', result.stdout)

    def test_extra_cmake_flags_without_cuda_host_compiler(self):
        result = subprocess.run(
            ['make', '-s', '-f', str(SCRIPTS.parent / 'Makefile.llamacpp-direct'),
             '--eval=print-flags:;@echo $(LLAMA_CPP_COMMON_CMAKE_FLAGS)', 'print-flags',
             'LLAMA_CUDA_HOST_COMPILER=', 'LLAMA_CPP_EXTRA_CMAKE_FLAGS=-DGGML_METAL_EMBED_LIBRARY=ON'],
            capture_output=True, text=True, check=True)
        self.assertIn('-DGGML_METAL_EMBED_LIBRARY=ON', result.stdout)

    def test_exact_sonames_and_transitive_closure(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            package = root / 'package'
            package.mkdir()
            libraries = root / 'libraries'
            libraries.mkdir()
            for name, source, flags in [
                ('libfixture_child.so.1', 'int child(void) { return 1; }', []),
                ('libfixture_parent.so.1', 'extern int child(void); int parent(void) { return child(); }',
                 ['-L' + str(libraries), '-l:libfixture_child.so.1']),
            ]:
                subprocess.run(['cc', '-shared', '-fPIC', '-x', 'c', '-',
                                '-Wl,-soname,' + name, '-o', str(libraries / name), *flags],
                               input=source, text=True, check=True)
            subprocess.run(['cc', '-shared', '-fPIC', '-x', 'c', '-', '-o', str(package / 'modeld.bin'),
                            '-L' + str(libraries), '-l:libfixture_parent.so.1'],
                           input='extern int parent(void); int entry(void) { return parent(); }',
                           text=True, check=True)
            (package / 'libfixture_parent.so.2').write_bytes((libraries / 'libfixture_parent.so.1').read_bytes())
            tools = root / 'tools'
            tools.mkdir()
            resolver = tools / 'ldconfig'
            resolver.write_text('#!/bin/sh\nprintf "%s\\n" "libfixture_parent.so.1 (libc6) => ' +
                                str(libraries / 'libfixture_parent.so.1') + '" "libfixture_child.so.1 (libc6) => ' +
                                str(libraries / 'libfixture_child.so.1') + '"\n')
            resolver.chmod(0o755)
            env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'])
            gate = ['bash', str(SCRIPTS / 'modeld-abi-check.sh'), str(package), '99', '99']
            self.assertNotEqual(subprocess.run(gate, capture_output=True).returncode, 0)
            subprocess.run(['bash', str(SCRIPTS / 'modeld-vendor-runtime-deps.sh'), str(package)],
                           env=env, check=True, capture_output=True)
            self.assertTrue((package / 'modeld-libs/libfixture_parent.so.1').is_file())
            self.assertTrue((package / 'modeld-libs/libfixture_child.so.1').is_file())
            subprocess.run(gate, check=True, capture_output=True)
            (package / 'worker-link').symlink_to('modeld.bin')
            report = subprocess.run(['python3', str(SCRIPTS / 'modeld-package-inventory.py'), str(package)],
                                    capture_output=True, text=True, check=True)
            files = {entry['path']: entry for entry in json.loads(report.stdout)['files']}
            self.assertIn('libfixture_parent.so.1', files['modeld.bin']['needed'])
            self.assertEqual(files['worker-link']['target'], 'modeld.bin')
            self.assertEqual(len(files['modeld.bin']['sha256']), 64)

    def test_empty_package_fails(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = subprocess.run(['bash', str(SCRIPTS / 'modeld-abi-check.sh'), temporary, '99', '99'],
                                    capture_output=True)
            self.assertNotEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()
