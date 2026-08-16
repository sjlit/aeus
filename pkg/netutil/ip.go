package netutil

import (
	"net"
	"strings"
)

// LocalIP returns the machine's first non-loopback IPv4 address,
// or "localhost" if none can be determined.
func LocalIP() string {
	var (
		err    error
		addrs  []net.Addr
		inters []net.Interface
	)
	if inters, err = net.Interfaces(); err != nil {
		return ""
	}
	for _, inter := range inters {
		if inter.Flags&net.FlagUp != net.FlagUp {
			continue
		}
		if !strings.HasPrefix(inter.Name, "lo") {
			if addrs, err = inter.Addrs(); err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
					if ipNet.IP.To4() != nil {
						return ipNet.IP.String()
					}
				}
			}
		}
	}
	return "localhost"
}
