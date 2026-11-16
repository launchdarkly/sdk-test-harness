package mockld

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// TruncatedBodyHandler returns an HTTP handler that responds with status 200 and a Content-Length
// header for the full body, but writes only the first half of the body and then closes the
// connection. The client sees a premature end of the response body (an I/O error while reading it).
func TruncatedBodyHandler(contentType string, body []byte) http.Handler {
	return brokenBodyHandler(contentType, body, false)
}

// ResetMidBodyHandler returns an HTTP handler that responds with status 200 and a Content-Length
// header for the full body, writes the first half of the body, and then resets the TCP connection
// (an RST rather than a normal close). The client sees a "connection reset" error while reading the
// response body.
func ResetMidBodyHandler(contentType string, body []byte) http.Handler {
	return brokenBodyHandler(contentType, body, true)
}

func brokenBodyHandler(contentType string, body []byte, reset bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			panic("brokenBodyHandler requires a ResponseWriter that supports Hijack")
		}
		conn, bufrw, err := hijacker.Hijack()
		if err != nil {
			panic(fmt.Sprintf("brokenBodyHandler could not hijack the connection: %s", err))
		}
		head := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n",
			contentType, len(body))
		_, _ = bufrw.WriteString(head)
		_, _ = bufrw.Write(body[:len(body)/2])
		_ = bufrw.Flush()
		// Give the client a moment to start reading the body, so the failure happens mid-body
		// rather than before the response headers have been processed.
		time.Sleep(time.Millisecond * 50)
		if reset {
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				_ = tcpConn.SetLinger(0) // makes Close send RST instead of FIN
			}
		}
		_ = conn.Close()
	})
}
