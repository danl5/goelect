package rpc

import (
	"errors"
)

type Config struct {
	// ServerCA defines the set of root certificate authorities
	// that servers use if required to verify a client certificate
	// by the policy in ClientAuth.
	ServerCAs        []string `json:"server_cas"`
	ServerKey        string   `json:"server_key"`
	ServerCert       string   `json:"server_cert"`
	ServerSkipVerify bool     `json:"server_skip_verify"`

	// ClientCAs defines the set of root certificate authorities
	// that clients use when verifying server certificates.
	// If ClientCAs is nil, TLS uses the host's root CA set.
	ClientCAs        []string `json:"client_cas"`
	ClientCert       string   `json:"client_cert"`
	ClientKey        string   `json:"client_key"`
	ClientSkipVerify bool     `json:"client_skip_verify"`
	// ConnectTimeout is the maximum amount of time a dial will wait for
	// a createClient to complete, in seconds.
	ConnectTimeout uint `json:"connect_timeout"`
}

func (c *Config) Validate() error {
	cfgCount := 0
	if c.ServerKey != "" {
		cfgCount++
	}
	if c.ServerCert != "" {
		cfgCount++
	}

	if cfgCount == 1 {
		return errors.New("incomplete server certificate configuration")
	}

	// if the server uses TLS, and not skip verification, we need to have server CAs
	if cfgCount == 2 && !c.ServerSkipVerify {
		if len(c.ServerCAs) == 0 {
			return errors.New("no server CAs configured")
		}
	}

	cfgCount = 0
	if c.ClientKey != "" {
		cfgCount++
	}
	if c.ClientCert != "" {
		cfgCount++
	}

	if cfgCount == 1 {
		return errors.New("incomplete client certificate configuration")
	}

	// if the client uses TLS, and not skip verification, we need to have client CAs
	if cfgCount == 2 && !c.ClientSkipVerify {
		if len(c.ClientCAs) == 0 {
			return errors.New("no client CAs configured")
		}
	}

	return nil
}
