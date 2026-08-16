package netutil

import (
	"net"
	"strconv"
)

// EffectiveAddr returns the actual listener address, falling back to LocalIP
// when addr is a wildcard (0.0.0.0, ::) and the listener provides no real host.
func EffectiveAddr(addr string, l net.Listener) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil && l == nil {
		return ""
	}
	if l != nil {
		if taddr, ok := l.Addr().(*net.TCPAddr); ok {
			host = taddr.IP.String()
			port = strconv.Itoa(taddr.Port)
		} else {
			if lh, lp, lerr := net.SplitHostPort(l.Addr().String()); lerr == nil {
				host = lh
				port = lp
			}
		}
	}
	if len(host) > 0 && (host != "0.0.0.0" && host != "[::]" && host != "::") {
		return net.JoinHostPort(host, port)
	}
	host = LocalIP()
	return net.JoinHostPort(host, port)
}
