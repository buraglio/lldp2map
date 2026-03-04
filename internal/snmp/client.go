package snmp

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// AuthProto represents SNMPv3 authentication protocol options.
type AuthProto string

// PrivProto represents SNMPv3 privacy protocol options.
type PrivProto string

const (
	AuthMD5    AuthProto = "MD5"
	AuthSHA    AuthProto = "SHA"
	AuthSHA256 AuthProto = "SHA256"
	AuthSHA512 AuthProto = "SHA512"

	PrivDES    PrivProto = "DES"
	PrivAES    PrivProto = "AES"
	PrivAES192 PrivProto = "AES192"
	PrivAES256 PrivProto = "AES256"
)

// Config holds SNMP connection parameters for v2c and v3.
type Config struct {
	Host    string
	Port    uint16
	Version string // "2c" or "3"
	Timeout time.Duration
	Retries int

	// v2c
	Community string

	// v3
	Username  string
	AuthProto AuthProto
	AuthPass  string
	PrivProto PrivProto
	PrivPass  string
	SecLevel  string // "noauth", "auth", "authpriv"
}

// Client wraps gosnmp with a simplified interface.
type Client struct {
	handler *gosnmp.GoSNMP
}

// New creates and connects a new SNMP client.
func New(cfg Config) (*Client, error) {
	// gosnmp dials "%s:%d" so bare IPv6 addresses need bracket notation.
	target := cfg.Host
	if ip := net.ParseIP(target); ip != nil && ip.To4() == nil && !strings.HasPrefix(target, "[") {
		target = "[" + target + "]"
	}

	g := &gosnmp.GoSNMP{
		Target:  target,
		Port:    cfg.Port,
		Timeout: cfg.Timeout,
		Retries: cfg.Retries,
		MaxOids: gosnmp.MaxOids,
	}

	if g.Port == 0 {
		g.Port = 161
	}
	if g.Timeout == 0 {
		g.Timeout = 5 * time.Second
	}
	if g.Retries == 0 {
		g.Retries = 2
	}

	switch cfg.Version {
	case "2c", "":
		g.Version = gosnmp.Version2c
		g.Community = cfg.Community
	case "3":
		g.Version = gosnmp.Version3
		g.SecurityModel = gosnmp.UserSecurityModel

		params := &gosnmp.UsmSecurityParameters{
			UserName: cfg.Username,
		}

		switch cfg.SecLevel {
		case "auth":
			g.MsgFlags = gosnmp.AuthNoPriv
			params.AuthenticationPassphrase = cfg.AuthPass
			params.AuthenticationProtocol = toAuthProto(cfg.AuthProto)
		case "authpriv":
			g.MsgFlags = gosnmp.AuthPriv
			params.AuthenticationPassphrase = cfg.AuthPass
			params.AuthenticationProtocol = toAuthProto(cfg.AuthProto)
			params.PrivacyPassphrase = cfg.PrivPass
			params.PrivacyProtocol = toPrivProto(cfg.PrivProto)
		default:
			g.MsgFlags = gosnmp.NoAuthNoPriv
		}

		g.SecurityParameters = params
	default:
		return nil, fmt.Errorf("unsupported SNMP version %q (use 2c or 3)", cfg.Version)
	}

	if err := g.Connect(); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", cfg.Host, err)
	}

	return &Client{handler: g}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() {
	if c.handler.Conn != nil {
		c.handler.Conn.Close()
	}
}

// Walk performs a BulkWalk of the given OID subtree, falling back to Walk on error.
func (c *Client) Walk(oid string) ([]gosnmp.SnmpPDU, error) {
	var pdus []gosnmp.SnmpPDU

	err := c.handler.BulkWalk(oid, func(pdu gosnmp.SnmpPDU) error {
		pdus = append(pdus, pdu)
		return nil
	})
	if err != nil {
		// Fallback for devices that don't support BulkWalk
		pdus = nil
		err = c.handler.Walk(oid, func(pdu gosnmp.SnmpPDU) error {
			pdus = append(pdus, pdu)
			return nil
		})
	}

	return pdus, err
}

// Get retrieves a single OID value.
func (c *Client) Get(oid string) (*gosnmp.SnmpPDU, error) {
	result, err := c.handler.Get([]string{oid})
	if err != nil {
		return nil, err
	}
	if len(result.Variables) == 0 {
		return nil, fmt.Errorf("no result for OID %s", oid)
	}
	pdu := result.Variables[0]
	return &pdu, nil
}

func toAuthProto(p AuthProto) gosnmp.SnmpV3AuthProtocol {
	switch p {
	case AuthSHA:
		return gosnmp.SHA
	case AuthSHA256:
		return gosnmp.SHA256
	case AuthSHA512:
		return gosnmp.SHA512
	default:
		return gosnmp.MD5
	}
}

func toPrivProto(p PrivProto) gosnmp.SnmpV3PrivProtocol {
	switch p {
	case PrivAES:
		return gosnmp.AES
	case PrivAES192:
		return gosnmp.AES192
	case PrivAES256:
		return gosnmp.AES256
	default:
		return gosnmp.DES
	}
}
