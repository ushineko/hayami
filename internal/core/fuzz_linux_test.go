package core

import "testing"

// FuzzParseInetDiag: one sock_diag reply, from the kernel or anything in
// its place (spec 074).
func FuzzParseInetDiag(f *testing.F) {
	msg := make([]byte, inetDiagMsgLen+4+tcpInfoBytesReceived+8)
	msg[msgFamily] = 2
	msg[msgSrc], msg[msgDst] = 192, 198
	msg[inetDiagMsgLen] = byte(4 + tcpInfoBytesReceived + 8)
	msg[inetDiagMsgLen+2] = inetDiagInfo
	f.Add(msg)
	f.Add([]byte{})
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = parseInetDiag(data)
	})
}
