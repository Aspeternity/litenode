package text

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestNetReason(t *testing.T) {
	dial := func(err error) error {
		return &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", err)}
	}
	tests := []struct {
		err  error
		want string
	}{
		{dial(syscall.ECONNREFUSED), "连接被拒绝"},
		{dial(syscall.ENETUNREACH), "网络不可达"},
		{dial(syscall.EHOSTUNREACH), "网络不可达"},
		{dial(syscall.ECONNRESET), "连接被对方断开"},
		{&net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}, "连接超时"},
		{context.DeadlineExceeded, "连接超时"},
		{io.EOF, "连接被对方断开"},
		{&net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}, "域名不存在"},
		{&net.DNSError{Err: "i/o timeout", Name: "x.com", IsTimeout: true}, "域名解析超时"},
		{&net.DNSError{Err: "server misbehaving", Name: "x.com"}, "域名解析失败"},
		{&net.OpError{Op: "remote error", Err: errors.New("tls: handshake failure")}, "TLS 握手失败"},
		{errors.New("something else"), "something else"},
	}
	for _, tt := range tests {
		if got := NetReason(tt.err); !strings.Contains(got, tt.want) {
			t.Errorf("NetReason(%v) = %q, want it to contain %q", tt.err, got, tt.want)
		}
	}
}
