import socket
import threading
import sys

def pipe(src, dst):
    try:
        while True:
            data = src.recv(8192)
            if not data:
                break
            dst.sendall(data)
    except:
        pass
    finally:
        try:
            src.close()
        except:
            pass
        try:
            dst.close()
        except:
            pass

def handle(client):
    remote = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        remote.connect(('127.0.0.1', 9223))
    except Exception as e:
        print(f"Proxy error: Failed to connect to Chromium: {e}")
        sys.stdout.flush()
        client.close()
        return
    
    # Start two unidirectional pipes
    threading.Thread(target=pipe, args=(client, remote), daemon=True).start()
    pipe(remote, client)

def main():
    # Use port 9222 as defined in entrypoint.sh/Dockerfile
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(('0.0.0.0', 9222))
    s.listen(10)
    print("Python CDP Proxy listening on 0.0.0.0:9222 forwarding to 127.0.0.1:9223")
    sys.stdout.flush()
    while True:
        try:
            c, addr = s.accept()
            threading.Thread(target=handle, args=(c,), daemon=True).start()
        except KeyboardInterrupt:
            break
        except Exception as e:
            print(f"Accept error: {e}")
            sys.stdout.flush()

if __name__ == '__main__':
    main()
