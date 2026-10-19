package text

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"syscall"
)

// NetReason describes why a connection or lookup failed, in place of Go's
// raw error such as "dial tcp 1.2.3.4:443: i/o timeout". Errors it does
// not recognize are shown as they are.
func NetReason(err error) string {
	var dnsErr *net.DNSError
	var alert tls.AlertError
	var recordErr tls.RecordHeaderError
	var opErr *net.OpError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return "域名不存在或没有对应的地址"
	case errors.As(err, &dnsErr) && dnsErr.IsTimeout:
		return "域名解析超时"
	case errors.As(err, &dnsErr):
		return "域名解析失败"
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return "连接超时（地址不通，或被防火墙丢弃）"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "连接被拒绝（目标端口没有程序在监听，或被防火墙拒绝）"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "连接被对方断开"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.EADDRNOTAVAIL):
		return "网络不可达（本机没有到该地址的路由，如没有 IPv6）"
	case errors.As(err, &alert), errors.As(err, &recordErr), errors.As(err, &opErr) && opErr.Op == "remote error":
		return "TLS 握手失败（对方不是 HTTPS 网站或不接受该域名）"
	}
	return err.Error()
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
