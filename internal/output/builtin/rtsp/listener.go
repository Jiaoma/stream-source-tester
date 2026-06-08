package rtsp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
)

type sharedListener struct {
	addr     string
	listener net.Listener
	mounts   map[string]*mountHandler
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

type mountHandler struct {
	mountPath string
	handler   func(conn net.Conn)
}

var (
	listenerRegistry = make(map[string]*sharedListener)
	registryMu       sync.Mutex
)

func getOrCreateListener(ctx context.Context, addr string) (*sharedListener, error) {
	registryMu.Lock()
	defer registryMu.Unlock()

	for _, sl := range listenerRegistry {
		if sl.addr == addr {
			return sl, nil
		}
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", addr, err)
	}

	slCtx, cancel := context.WithCancel(ctx)
	actualAddr := listener.Addr().String()
	sl := &sharedListener{
		addr:     actualAddr,
		listener: listener,
		mounts:   make(map[string]*mountHandler),
		ctx:      slCtx,
		cancel:   cancel,
	}
	listenerRegistry[actualAddr] = sl

	go sl.acceptLoop()

	return sl, nil
}

func (sl *sharedListener) registerMount(mountPath string, handler func(conn net.Conn)) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.mounts[mountPath] = &mountHandler{
		mountPath: mountPath,
		handler:   handler,
	}
}

func (sl *sharedListener) unregisterMount(mountPath string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	delete(sl.mounts, mountPath)
	if len(sl.mounts) == 0 {
		sl.cancel()
		registryMu.Lock()
		delete(listenerRegistry, sl.addr)
		registryMu.Unlock()
	}
}

func (sl *sharedListener) acceptLoop() {
	defer sl.listener.Close()
	for {
		select {
		case <-sl.ctx.Done():
			return
		default:
		}
		conn, err := sl.listener.Accept()
		if err != nil {
			select {
			case <-sl.ctx.Done():
				return
			default:
				continue
			}
		}
		go sl.handleConnection(conn)
	}
}

func (sl *sharedListener) handleConnection(conn net.Conn) {
	peekBuf := make([]byte, 512)
	n, err := conn.Read(peekBuf)
	if err != nil {
		conn.Close()
		return
	}

	mountPath := ""
	for i := 0; i < n-7; i++ {
		if peekBuf[i] == ' ' && peekBuf[i+1] == 'r' && peekBuf[i+2] == 't' && peekBuf[i+3] == 's' && peekBuf[i+4] == 'p' {
			for j := i + 5; j < n; j++ {
				if peekBuf[j] == ' ' || peekBuf[j] == '\r' || peekBuf[j] == '\n' {
					uri := string(peekBuf[i+1 : j])
					mountPath = extractMountPath(uri)
					break
				}
			}
			break
		}
	}

	sl.mu.RLock()
	handler, ok := sl.mounts[mountPath]
	sl.mu.RUnlock()

	if !ok {
		_, _ = conn.Write([]byte("RTSP/1.0 404 Not Found\r\nCSeq: 1\r\n\r\n"))
		conn.Close()
		return
	}

	wrappedConn := &prependConn{
		Conn:   conn,
		prefix: peekBuf[:n],
		offset: 0,
	}
	handler.handler(wrappedConn)
}

type prependConn struct {
	net.Conn
	prefix []byte
	offset int
}

func (p *prependConn) Read(b []byte) (n int, err error) {
	if p.offset < len(p.prefix) {
		n = copy(b, p.prefix[p.offset:])
		p.offset += n
		return n, nil
	}
	return p.Conn.Read(b)
}

func extractMountPath(uri string) string {
	if parsed, err := url.Parse(uri); err == nil && parsed.Path != "" {
		path := strings.TrimPrefix(parsed.Path, "/")
		if slash := strings.IndexByte(path, '/'); slash >= 0 {
			return path[:slash]
		}
		return path
	}
	if strings.HasPrefix(uri, "rtsp://") {
		withoutScheme := strings.TrimPrefix(uri, "rtsp://")
		if slash := strings.IndexByte(withoutScheme, '/'); slash >= 0 {
			uri = withoutScheme[slash+1:]
		} else {
			uri = ""
		}
	}
	uri = strings.TrimPrefix(uri, "/")
	if slash := strings.IndexByte(uri, '/'); slash >= 0 {
		return uri[:slash]
	}
	return uri
}
