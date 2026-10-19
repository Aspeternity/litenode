# Development helper (not installed): send one DNS query (A example.com)
# over UDP to <host> <port> and print "udp=ok" if an answer comes back.
#   python3 -I scripts/dev/dnsprobe.py 127.0.0.1 10853
import socket, sys
q = b"\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01"
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(5)
try:
    s.sendto(q, (sys.argv[1], int(sys.argv[2])))
    data, _ = s.recvfrom(512)
    print("udp=ok" if data[:2] == b"\x12\x34" and data[7] > 0 else "udp=FAILED")
except OSError:
    print("udp=FAILED")
