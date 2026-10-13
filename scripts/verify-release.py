import pathlib, subprocess, sys
root=pathlib.Path(sys.argv[1]).resolve()
for name in ['vmbench','engines/fio','engines/stress-ng','engines/iperf3','engines/compute-bench']:
    p=root/name
    b=p.read_bytes()
    if b[:4]!=b'\x7fELF': raise SystemExit(f'{name}: not ELF')
    headers=subprocess.check_output(['readelf','-l',str(p)],text=True)
    dynamic=subprocess.check_output(['readelf','-d',str(p)],text=True)
    if 'INTERP' in headers or '(NEEDED)' in dynamic: raise SystemExit(f'{name}: release requires static engines')
    subprocess.run([str(p),'--version'],check=True,timeout=10)
print('All executables validated as static and runnable on build host; Ubuntu target smoke checks remain required.')
