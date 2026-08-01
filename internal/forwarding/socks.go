package forwarding

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// SOCKS5 protocol constants (RFC 1928).
const (
	socksVersion = 0x05

	socksNoAuth       = 0x00
	socksNoAcceptable = 0xFF

	socksCmdConnect = 0x01

	socksAtypIPv4   = 0x01
	socksAtypDomain = 0x03
	socksAtypIPv6   = 0x04

	socksRepSuccess          = 0x00
	socksRepGeneralFailure   = 0x01
	socksRepCmdNotSupported  = 0x07
	socksRepAtypNotSupported = 0x08
)

// socksHandshake performs the server side of a SOCKS5 negotiation on conn,
// supporting the no-authentication method and the CONNECT command only. On
// success it writes the success reply and returns the requested "host:port"
// destination; on a protocol or unsupported-feature error it writes the
// appropriate failure reply (best effort) and returns an error. The dial to
// the destination is performed by the caller.
func socksHandshake(conn net.Conn) (string, error) {
	if err := socksNegotiateAuth(conn); err != nil {
		return "", fmt.Errorf("socks auth: %w", err)
	}

	// Request: VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT.
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", fmt.Errorf("socks request header: %w", err)
	}
	if header[0] != socksVersion {
		return "", fmt.Errorf("socks: unexpected request version 0x%02x", header[0])
	}
	if header[1] != socksCmdConnect {
		socksReplyFailure(conn, socksRepCmdNotSupported)
		return "", fmt.Errorf("socks: unsupported command 0x%02x", header[1])
	}

	host, err := socksReadAddress(conn, header[3])
	if err != nil {
		socksReplyFailure(conn, socksRepAtypNotSupported)
		return "", err
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", fmt.Errorf("socks port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBytes)

	if err := socksReplySuccess(conn); err != nil {
		return "", fmt.Errorf("socks reply: %w", err)
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func socksNegotiateAuth(conn net.Conn) error {
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return err
	}
	if greeting[0] != socksVersion {
		return fmt.Errorf("unexpected version 0x%02x", greeting[0])
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	for _, m := range methods {
		if m == socksNoAuth {
			_, err := conn.Write([]byte{socksVersion, socksNoAuth})
			return err
		}
	}
	_, _ = conn.Write([]byte{socksVersion, socksNoAcceptable})
	return fmt.Errorf("no acceptable auth method offered")
}

func socksReadAddress(conn net.Conn, atyp byte) (string, error) {
	switch atyp {
	case socksAtypIPv4:
		buf := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", fmt.Errorf("socks ipv4 addr: %w", err)
		}
		return net.IP(buf).String(), nil
	case socksAtypIPv6:
		buf := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", fmt.Errorf("socks ipv6 addr: %w", err)
		}
		return net.IP(buf).String(), nil
	case socksAtypDomain:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return "", fmt.Errorf("socks domain length: %w", err)
		}
		name := make([]byte, int(lenByte[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			return "", fmt.Errorf("socks domain: %w", err)
		}
		return string(name), nil
	default:
		return "", fmt.Errorf("socks: unsupported address type 0x%02x", atyp)
	}
}

// socksReplySuccess writes a reply with a zeroed BND.ADDR/BND.PORT, which
// clients accept — the bound address is only informational for CONNECT.
func socksReplySuccess(conn net.Conn) error {
	_, err := conn.Write([]byte{socksVersion, socksRepSuccess, 0x00, socksAtypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

func socksReplyFailure(conn net.Conn, code byte) {
	_, _ = conn.Write([]byte{socksVersion, code, 0x00, socksAtypIPv4, 0, 0, 0, 0, 0, 0})
}
