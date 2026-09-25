import argparse
import hashlib
import json
import pathlib
import subprocess


def inventory(root):
    entries = []
    for path in sorted(root.rglob('*')):
        entry = {'path': str(path.relative_to(root))}
        if path.is_symlink():
            entry.update(kind='symlink', target=str(path.readlink()))
        elif path.is_file():
            digest = hashlib.sha256()
            with path.open('rb') as stream:
                magic = stream.read(4)
                digest.update(magic)
                for block in iter(lambda: stream.read(1024 * 1024), b''):
                    digest.update(block)
            entry.update(kind='file', size=path.stat().st_size, sha256=digest.hexdigest())
            if magic == b'\x7fELF':
                dynamic = subprocess.run(['objdump', '-p', str(path)], check=True,
                                         capture_output=True, text=True).stdout
                for field in ('NEEDED', 'SONAME', 'RPATH', 'RUNPATH'):
                    entry[field.lower()] = [line.split(None, 1)[1] for line in dynamic.splitlines()
                                            if line.strip().startswith(field + ' ')]
        else:
            continue
        entries.append(entry)
    return {'schema': 1, 'files': entries}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Inventory a Linux modeld package without executing it.')
    parser.add_argument('package', type=pathlib.Path)
    args = parser.parse_args()
    if not args.package.is_dir():
        parser.error('package must be a directory')
    print(json.dumps(inventory(args.package), indent=2))
