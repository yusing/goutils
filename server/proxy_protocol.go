package server

import (
	"fmt"
	"net"

	"github.com/pires/go-proxyproto"
)

// ProxyProtocolMode selects how a listener handles incoming PROXY protocol
// connections.
type ProxyProtocolMode string

const (
	// ProxyProtocolModeDisabled leaves every connection unwrapped.
	ProxyProtocolModeDisabled ProxyProtocolMode = "disabled"
	// ProxyProtocolModeMixed requires headers from trusted proxies and treats
	// every other peer as a direct client without parsing its bytes.
	ProxyProtocolModeMixed ProxyProtocolMode = "mixed"
	// ProxyProtocolModeRequired accepts only trusted proxies and requires each
	// connection to provide a valid header.
	ProxyProtocolModeRequired ProxyProtocolMode = "required"
)

// ProxyProtocolConfig configures durable, source-authenticated PROXY protocol
// handling.
type ProxyProtocolConfig struct {
	Mode           ProxyProtocolMode `json:"mode"`
	TrustedProxies []string          `json:"trusted_proxies,omitempty"`
}

// ProxyProtocolPolicy determines which TCP peers may provide PROXY headers.
type ProxyProtocolPolicy struct {
	connPolicy proxyproto.ConnPolicyFunc
	configured bool
}

// Validate rejects unknown modes, enabled modes without trusted proxies, and
// invalid trusted IP addresses or CIDR ranges.
func (cfg ProxyProtocolConfig) Validate() error {
	_, err := newProxyProtocolConnPolicy(cfg)
	return err
}

// NewProxyProtocolPolicy builds the durable policy described by cfg.
func NewProxyProtocolPolicy(cfg ProxyProtocolConfig) (ProxyProtocolPolicy, error) {
	connPolicy, err := newProxyProtocolConnPolicy(cfg)
	if err != nil {
		return ProxyProtocolPolicy{}, err
	}
	return ProxyProtocolPolicy{connPolicy: connPolicy, configured: true}, nil
}

func newProxyProtocolConnPolicy(cfg ProxyProtocolConfig) (proxyproto.ConnPolicyFunc, error) {
	switch cfg.Mode {
	case ProxyProtocolModeDisabled:
		return nil, nil
	case ProxyProtocolModeMixed:
		if len(cfg.TrustedProxies) == 0 {
			return nil, fmt.Errorf("proxy protocol mode %q requires at least one trusted proxy", cfg.Mode)
		}
		return proxyproto.PolicyFromRanges(cfg.TrustedProxies, proxyproto.REQUIRE, proxyproto.SKIP)
	case ProxyProtocolModeRequired:
		if len(cfg.TrustedProxies) == 0 {
			return nil, fmt.Errorf("proxy protocol mode %q requires at least one trusted proxy", cfg.Mode)
		}
		return proxyproto.TrustProxyHeaderFromRanges(cfg.TrustedProxies)
	case "":
		return nil, fmt.Errorf("proxy protocol mode is required")
	default:
		return nil, fmt.Errorf("unknown proxy protocol mode %q", cfg.Mode)
	}
}

// NewLegacyProxyProtocolPolicy preserves the deprecated behavior where a
// PROXY header is optional and accepted from any peer.
func NewLegacyProxyProtocolPolicy() ProxyProtocolPolicy {
	return ProxyProtocolPolicy{connPolicy: useProxyProtocolHeader, configured: true}
}

func useProxyProtocolHeader(proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
	return proxyproto.USE, nil
}

// Enabled reports whether the policy wraps and parses listener connections.
func (p ProxyProtocolPolicy) Enabled() bool {
	return p.connPolicy != nil
}

// Wrap applies the policy to listener.
func (p ProxyProtocolPolicy) Wrap(listener net.Listener) net.Listener {
	if !p.Enabled() {
		return listener
	}
	return &proxyproto.Listener{
		Listener:   listener,
		ConnPolicy: p.connPolicy,
	}
}
