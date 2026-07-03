// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// NodeFirewallOptions models the per-node firewall options at
// /nodes/{node}/firewall/options.
type NodeFirewallOptions struct {
	Enable                           proxmoxOptionalBool
	LogLevelIn                       string
	LogLevelOut                      string
	LogLevelForward                  string
	LogNFConntrack                   proxmoxOptionalBool
	NFConntrackAllowInvalid          proxmoxOptionalBool
	NFConntrackMax                   proxmoxOptionalInt64
	NFConntrackTCPTimeoutEstablished proxmoxOptionalInt64
	NFConntrackTCPSynRecvTimeout     proxmoxOptionalInt64
	NFConntrackHelpers               string
	Ndp                              proxmoxOptionalBool
	Nosmurfs                         proxmoxOptionalBool
	ProtectionSynflood               proxmoxOptionalBool
	ProtectionSynfloodBurst          proxmoxOptionalInt64
	ProtectionSynfloodRate           proxmoxOptionalInt64
	SmurfLogLevel                    string
	TCPFlagsLogLevel                 string
	TCPFlags                         proxmoxOptionalBool
	Nftables                         proxmoxOptionalBool
}

type nodeFirewallOptionsKnown struct {
	Enable                           proxmoxOptionalBool  `json:"enable"`
	LogLevelIn                       string               `json:"log_level_in"`
	LogLevelOut                      string               `json:"log_level_out"`
	LogLevelForward                  string               `json:"log_level_forward"`
	LogNFConntrack                   proxmoxOptionalBool  `json:"log_nf_conntrack"`
	NFConntrackAllowInvalid          proxmoxOptionalBool  `json:"nf_conntrack_allow_invalid"`
	NFConntrackMax                   proxmoxOptionalInt64 `json:"nf_conntrack_max"`
	NFConntrackTCPTimeoutEstablished proxmoxOptionalInt64 `json:"nf_conntrack_tcp_timeout_established"`
	NFConntrackTCPSynRecvTimeout     proxmoxOptionalInt64 `json:"nf_conntrack_tcp_timeout_syn_recv"`
	NFConntrackHelpers               string               `json:"nf_conntrack_helpers"`
	Ndp                              proxmoxOptionalBool  `json:"ndp"`
	Nosmurfs                         proxmoxOptionalBool  `json:"nosmurfs"`
	ProtectionSynflood               proxmoxOptionalBool  `json:"protection_synflood"`
	ProtectionSynfloodBurst          proxmoxOptionalInt64 `json:"protection_synflood_burst"`
	ProtectionSynfloodRate           proxmoxOptionalInt64 `json:"protection_synflood_rate"`
	SmurfLogLevel                    string               `json:"smurf_log_level"`
	TCPFlagsLogLevel                 string               `json:"tcp_flags_log_level"`
	TCPFlags                         proxmoxOptionalBool  `json:"tcpflags"`
	Nftables                         proxmoxOptionalBool  `json:"nftables"`
}

type NodeFirewallOptionsRequest struct {
	Enable                           *bool
	LogLevelIn                       *string
	LogLevelOut                      *string
	LogLevelForward                  *string
	LogNFConntrack                   *bool
	NFConntrackAllowInvalid          *bool
	NFConntrackMax                   *int64
	NFConntrackTCPTimeoutEstablished *int64
	NFConntrackTCPSynRecvTimeout     *int64
	NFConntrackHelpers               *string
	Ndp                              *bool
	Nosmurfs                         *bool
	ProtectionSynflood               *bool
	ProtectionSynfloodBurst          *int64
	ProtectionSynfloodRate           *int64
	SmurfLogLevel                    *string
	TCPFlagsLogLevel                 *string
	TCPFlags                         *bool
	Nftables                         *bool
	Delete                           []string
}

func (c *Client) GetNodeFirewallOptions(ctx context.Context, node string) (NodeFirewallOptions, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/firewall/options", url.PathEscape(node)), nil, nil, &raw); err != nil {
		return NodeFirewallOptions{}, err
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return NodeFirewallOptions{}, fmt.Errorf("unable to marshal raw firewall options: %w", err)
	}
	var known nodeFirewallOptionsKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return NodeFirewallOptions{}, fmt.Errorf("unable to decode firewall options: %w", err)
	}
	return NodeFirewallOptions{
		Enable:                           known.Enable,
		LogLevelIn:                       known.LogLevelIn,
		LogLevelOut:                      known.LogLevelOut,
		LogLevelForward:                  known.LogLevelForward,
		LogNFConntrack:                   known.LogNFConntrack,
		NFConntrackAllowInvalid:          known.NFConntrackAllowInvalid,
		NFConntrackMax:                   known.NFConntrackMax,
		NFConntrackTCPTimeoutEstablished: known.NFConntrackTCPTimeoutEstablished,
		NFConntrackTCPSynRecvTimeout:     known.NFConntrackTCPSynRecvTimeout,
		NFConntrackHelpers:               known.NFConntrackHelpers,
		Ndp:                              known.Ndp,
		Nosmurfs:                         known.Nosmurfs,
		ProtectionSynflood:               known.ProtectionSynflood,
		ProtectionSynfloodBurst:          known.ProtectionSynfloodBurst,
		ProtectionSynfloodRate:           known.ProtectionSynfloodRate,
		SmurfLogLevel:                    known.SmurfLogLevel,
		TCPFlagsLogLevel:                 known.TCPFlagsLogLevel,
		TCPFlags:                         known.TCPFlags,
		Nftables:                         known.Nftables,
	}, nil
}

func (c *Client) UpdateNodeFirewallOptions(ctx context.Context, node string, req NodeFirewallOptionsRequest) error {
	form := url.Values{}
	setOptionalBool(form, "enable", req.Enable)
	setOptionalString(form, "log_level_in", req.LogLevelIn)
	setOptionalString(form, "log_level_out", req.LogLevelOut)
	setOptionalString(form, "log_level_forward", req.LogLevelForward)
	setOptionalBool(form, "log_nf_conntrack", req.LogNFConntrack)
	setOptionalBool(form, "nf_conntrack_allow_invalid", req.NFConntrackAllowInvalid)
	setOptionalInt64(form, "nf_conntrack_max", req.NFConntrackMax)
	setOptionalInt64(form, "nf_conntrack_tcp_timeout_established", req.NFConntrackTCPTimeoutEstablished)
	setOptionalInt64(form, "nf_conntrack_tcp_timeout_syn_recv", req.NFConntrackTCPSynRecvTimeout)
	setOptionalString(form, "nf_conntrack_helpers", req.NFConntrackHelpers)
	setOptionalBool(form, "ndp", req.Ndp)
	setOptionalBool(form, "nosmurfs", req.Nosmurfs)
	setOptionalBool(form, "protection_synflood", req.ProtectionSynflood)
	setOptionalInt64(form, "protection_synflood_burst", req.ProtectionSynfloodBurst)
	setOptionalInt64(form, "protection_synflood_rate", req.ProtectionSynfloodRate)
	setOptionalString(form, "smurf_log_level", req.SmurfLogLevel)
	setOptionalString(form, "tcp_flags_log_level", req.TCPFlagsLogLevel)
	setOptionalBool(form, "tcpflags", req.TCPFlags)
	setOptionalBool(form, "nftables", req.Nftables)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/firewall/options", url.PathEscape(node)), nil, form, nil)
}

func (c *Client) DeleteNodeFirewallOptions(ctx context.Context, node string) error {
	// There is no DELETE endpoint; reset by setting all known keys to their defaults
	// via delete.
	reset := NodeFirewallOptionsRequest{
		Delete: []string{
			"enable", "log_level_in", "log_level_out", "log_level_forward",
			"log_nf_conntrack", "nf_conntrack_allow_invalid", "nf_conntrack_max",
			"nf_conntrack_tcp_timeout_established", "nf_conntrack_tcp_timeout_syn_recv",
			"nf_conntrack_helpers", "ndp", "nosmurfs", "protection_synflood",
			"protection_synflood_burst", "protection_synflood_rate", "smurf_log_level",
			"tcp_flags_log_level", "tcpflags", "nftables",
		},
	}
	err := c.UpdateNodeFirewallOptions(ctx, node, reset)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
