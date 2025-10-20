// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PathSearchParams defines query options for path analysis.
type PathSearchParams struct {
	From                    string
	SrcIP                   string
	DstIP                   string
	Intent                  string
	SnapshotID              string
	IPProto                 *int
	SrcPort                 string
	DstPort                 string
	IcmpType                *int
	TCPFlags                PathTCPFlags
	AppID                   string
	UserID                  string
	UserGroupID             string
	URL                     string
	IncludeTags             *bool
	IncludeNetworkFunctions *bool
	MaxCandidates           *int
	MaxResults              *int
	MaxReturnPathResults    *int
	MaxSeconds              *int
}

// PathTCPFlags represents optional TCP flag filters.
type PathTCPFlags struct {
	FIN *int
	SYN *int
	RST *int
	PSH *int
	ACK *int
	URG *int
}

// PathSearchResult captures path analysis output.
type PathSearchResult struct {
	SrcIPLocationType string                `json:"srcIpLocationType"`
	DstIPLocationType string                `json:"dstIpLocationType"`
	Info              PathCollection        `json:"info"`
	ReturnPathInfo    PathCollection        `json:"returnPathInfo"`
	TimedOut          bool                  `json:"timedOut"`
	QueryURL          string                `json:"queryUrl"`
	Unrecognized      PathUnrecognizedValue `json:"unrecognizedValues"`
}

// PathCollection represents a set of paths with aggregation info.
type PathCollection struct {
	Paths     []Path `json:"paths"`
	TotalHits struct {
		Type  string `json:"type"`
		Value int64  `json:"value"`
	} `json:"totalHits"`
}

// PathUnrecognizedValue enumerates value mismatches returned by API.
type PathUnrecognizedValue struct {
	AppID       []string `json:"appId"`
	UserID      []string `json:"userId"`
	UserGroupID []string `json:"userGroupId"`
}

// Path represents a single path through the network.
type Path struct {
	ForwardingOutcome string    `json:"forwardingOutcome"`
	SecurityOutcome   string    `json:"securityOutcome"`
	Hops              []PathHop `json:"hops"`
}

// PathHop represents an individual hop in the path search.
type PathHop struct {
	DeviceName       string               `json:"deviceName"`
	DisplayName      string               `json:"displayName"`
	DeviceType       string               `json:"deviceType"`
	Tags             []string             `json:"tags"`
	ParseError       *bool                `json:"parseError"`
	IngressInterface string               `json:"ingressInterface"`
	EgressInterface  string               `json:"egressInterface"`
	Behaviors        []string             `json:"behaviors"`
	NetworkFunctions *PathNetworkFunction `json:"networkFunctions"`
	BackfilledFrom   string               `json:"backfilledFrom"`
}

// PathNetworkFunction captures ACL and zone context for a hop.
type PathNetworkFunction struct {
	ACL     []PathACL           `json:"acl"`
	Ingress PathInterfaceDetail `json:"ingress"`
	Egress  PathInterfaceDetail `json:"egress"`
}

// PathACL describes ACL evaluation on a hop.
type PathACL struct {
	Name    string `json:"name"`
	Context string `json:"context"`
	Action  string `json:"action"`
}

// PathInterfaceDetail captures interface/zone information for ingress/egress.
type PathInterfaceDetail struct {
	L2           PathInterface `json:"l2"`
	L3           PathInterface `json:"l3"`
	SecurityZone string        `json:"securityZone"`
}

// PathInterface describes a layer interface context.
type PathInterface struct {
	InterfaceName string `json:"interfaceName"`
	VRF           string `json:"vrf"`
}

// SearchPaths executes a path analysis query.
func (c *Client) SearchPaths(ctx context.Context, networkID string, params PathSearchParams) (*PathSearchResult, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, fmt.Errorf("networkID must be provided")
	}

	if params.DstIP == "" {
		return nil, fmt.Errorf("dstIP must be provided")
	}

	if params.From == "" && params.SrcIP == "" {
		return nil, fmt.Errorf("either from or srcIp must be provided")
	}

	query := url.Values{}
	if params.From != "" {
		query.Set("from", params.From)
	}
	if params.SrcIP != "" {
		query.Set("srcIp", params.SrcIP)
	}
	query.Set("dstIp", params.DstIP)

	if params.Intent != "" {
		query.Set("intent", params.Intent)
	}
	if params.SnapshotID != "" {
		query.Set("snapshotId", params.SnapshotID)
	}

	addInt := func(key string, value *int) {
		if value != nil {
			query.Set(key, strconv.Itoa(*value))
		}
	}

	if params.IPProto != nil {
		query.Set("ipProto", strconv.Itoa(*params.IPProto))
	}
	if params.SrcPort != "" {
		query.Set("srcPort", params.SrcPort)
	}
	if params.DstPort != "" {
		query.Set("dstPort", params.DstPort)
	}
	addInt("icmpType", params.IcmpType)
	addInt("fin", params.TCPFlags.FIN)
	addInt("syn", params.TCPFlags.SYN)
	addInt("rst", params.TCPFlags.RST)
	addInt("psh", params.TCPFlags.PSH)
	addInt("ack", params.TCPFlags.ACK)
	addInt("urg", params.TCPFlags.URG)

	if params.AppID != "" {
		query.Set("appId", params.AppID)
	}
	if params.UserID != "" {
		query.Set("userId", params.UserID)
	}
	if params.UserGroupID != "" {
		query.Set("userGroupId", params.UserGroupID)
	}
	if params.URL != "" {
		query.Set("url", params.URL)
	}

	if params.IncludeTags != nil {
		query.Set("includeTags", strconv.FormatBool(*params.IncludeTags))
	}
	if params.IncludeNetworkFunctions != nil {
		query.Set("includeNetworkFunctions", strconv.FormatBool(*params.IncludeNetworkFunctions))
	}
	addInt("maxCandidates", params.MaxCandidates)
	addInt("maxResults", params.MaxResults)
	addInt("maxReturnPathResults", params.MaxReturnPathResults)
	addInt("maxSeconds", params.MaxSeconds)

	path := fmt.Sprintf("/api/networks/%s/paths", url.PathEscape(networkID))
	if enc := query.Encode(); enc != "" {
		path = path + "?" + enc
	}

	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute path search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d searching paths: %s", resp.StatusCode, string(body))
	}

	var result PathSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode path search response: %w", err)
	}

	return &result, nil
}
