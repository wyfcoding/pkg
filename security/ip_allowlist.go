package security

import (
	"net"
	"net/http"
	"strings"

	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

// IPAllowlistMiddleware 根据 IP 白名单过滤访问请求。
func IPAllowlistMiddleware(allowlist []string) gin.HandlerFunc {
	cidrs, ips := parseIPAllowlist(allowlist)

	return func(c *gin.Context) {
		if len(cidrs) == 0 && len(ips) == 0 {
			c.Next()
			return
		}

		ipStr := clientIP(c)
		if ipStr == "" {
			response.ErrorWithStatus(c, http.StatusForbidden, "access denied", "missing client ip")
			c.Abort()
			return
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			response.ErrorWithStatus(c, http.StatusForbidden, "access denied", "invalid client ip")
			c.Abort()
			return
		}

		if ipAllowed(ip, cidrs, ips) {
			c.Next()
			return
		}

		response.ErrorWithStatus(c, http.StatusForbidden, "access denied", "ip not allowed")
		c.Abort()
	}
}

func parseIPAllowlist(entries []string) ([]*net.IPNet, []net.IP) {
	cidrs := make([]*net.IPNet, 0)
	ips := make([]net.IP, 0)

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err == nil {
				cidrs = append(cidrs, network)
			}
			continue
		}
		ip := net.ParseIP(entry)
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	return cidrs, ips
}

func ipAllowed(ip net.IP, cidrs []*net.IPNet, ips []net.IP) bool {
	for _, allowed := range ips {
		if allowed.Equal(ip) {
			return true
		}
	}
	for _, network := range cidrs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func clientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.ClientIP()
}
