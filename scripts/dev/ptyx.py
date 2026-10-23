#!/usr/bin/env python3
# Development helper (not installed): drive a menu program through a pty,
# answering prompts by regex. Run on the test VPS:
#   python3 ptyx.py STEPS -- litenode
# STEPS lines: regex<TAB>input[<TAB>timeout]
#   input "@show" prints output since last send (no send); "@show:N" last N lines
#   input "^C" sends Ctrl+C; "@none" sends nothing (just wait for regex)
#   input "@sleep:N" sleeps N seconds (regex ignored)
#   input "@raw:TEXT" sends TEXT with Python escapes decoded (\x1b, \r),
#     no newline added: e.g. a bracketed paste
#   input "@has:TEXT" prints whether the raw output since the last send
#     contains TEXT (escapes decoded), e.g. \x1b[?2004h; sends nothing
import codecs, os, pty, re, select, sys, time

ANSI = re.compile(r'\x1b\[[0-9;?]*[A-Za-z]|\x1b[()][A-Z0-9]')

def clean(b):
    s = b.decode('utf-8', 'replace')
    s = ANSI.sub('', s).replace('\r', '')
    return s

steps_file = sys.argv[1]
cmd = sys.argv[sys.argv.index('--') + 1:]
steps = []
for line in open(steps_file, encoding='utf-8'):
    line = line.rstrip('\n')
    if not line or line.startswith('#'):
        continue
    parts = line.split('\t')
    while len(parts) < 3:
        parts.append('')
    steps.append((parts[0], parts[1], float(parts[2] or 30)))

pid, fd = pty.fork()
if pid == 0:
    os.environ['TERM'] = 'xterm'
    os.execvp(cmd[0], cmd)

buf = b''
def read_some(t):
    global buf
    r, _, _ = select.select([fd], [], [], t)
    if r:
        try:
            d = os.read(fd, 65536)
        except OSError:
            return False
        if not d:
            return False
        buf += d
    return True

rc = 0
for pat, inp, tmo in steps:
    if inp.startswith('@sleep:'):
        end = time.time() + float(inp[7:])
        while time.time() < end:
            read_some(0.2)
        continue
    rx = re.compile(pat) if pat else None
    end = time.time() + tmo
    ok = rx is None
    while not ok and time.time() < end:
        alive = read_some(0.2)
        if rx.search(clean(buf)):
            ok = True
            break
        if not alive:
            break
    # let output settle briefly
    t2 = time.time() + 0.3
    while time.time() < t2:
        read_some(0.1)
    if not ok:
        print('[TIMEOUT] waiting for %r; last output:' % pat)
        print('\n'.join(clean(buf).splitlines()[-15:]))
        rc = 1
        break
    if inp.startswith('@show'):
        n = int(inp.split(':')[1]) if ':' in inp else 40
        lines = [l for l in clean(buf).splitlines() if l.strip() and not set(l.strip()) <= set('─═╔╗╚╝')]
        print('\n'.join(lines[-n:]))
        print('-----')
        buf = b''
        continue
    if inp.startswith('@has:'):
        want = codecs.decode(inp[5:], 'unicode_escape').encode('latin-1')
        print('[has] %r: %s' % (want, 'yes' if want in buf else 'no'))
        continue
    print('[ok] %s => %s' % (pat, inp))
    buf = b''
    if inp == '@none':
        continue
    if inp.startswith('@raw:'):
        os.write(fd, codecs.decode(inp[5:], 'unicode_escape').encode('latin-1'))
        continue
    if inp == '^C':
        os.write(fd, b'\x03')
    else:
        os.write(fd, inp.encode() + b'\n')

# drain
end = time.time() + 2
while time.time() < end and read_some(0.2):
    pass
try:
    os.kill(pid, 9)
except OSError:
    pass
sys.exit(rc)
