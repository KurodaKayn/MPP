import socket
import threading
import select
import sys

def proxy(src, dst):
    try:
        while True:
            r, _, _ = select.select([src, dst], [], [])
            for s in r:
                data = s.recv(8192)
                if not data: return
                (dst if s is src else src).sendall(data)
    except:
        pass
    finally:
        src.close()
        dst.close()

def handle(client):
    remote = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        remote.connect(('127.0.0.1', 9222))
    except:
        client.close()
        return
    threading.Thread(target=proxy, args=(client, remote), daemon=True).start()
    proxy(remote, client)

def main():
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(('0.0.0.0', 9223))
    s.listen(5)
    print("Python CDP Proxy listening on 0.0.0.0:9223 forwarding to 127.0.0.1:9222")
    sys.stdout.flush()
    while True:
        c, _ = s.accept()
        threading.Thread(target=handle, args=(c,), daemon=True).start()

if __name__ == '__main__':
    main()
