package model

import "errors"

var (
	ErrorBadCommand = errors.New("bad command")
)

// HeartBeatRequest is the heartbeat request
type HeartBeatRequest struct {
	NodeId string `json:"node_id" mapstructure:"node_id"`
	Term   uint64 `json:"term" mapstructure:"term"`
}

// HeartBeatResponse is the heartbeat response
type HeartBeatResponse struct {
	Ok      bool   `json:"ok,omitempty" mapstructure:"ok"`
	Message string `json:"message,omitempty" mapstructure:"message"`
}

func HBResponse(resp *HeartBeatResponse, ok bool, msg string) {
	resp.Ok = ok
	resp.Message = msg
}

// RequestVoteRequest is the vote request
type RequestVoteRequest struct {
	NodeId   string `json:"node_id" mapstructure:"node_id"`
	Term     uint64 `json:"term" mapstructure:"term"`
	NodeAddr string `json:"node_addr" mapstructure:"node_addr"`
}

// RequestVoteResponse is the vote response
type RequestVoteResponse struct {
	Node    Node   `json:"node" mapstructure:"node"`
	Vote    bool   `json:"vote" mapstructure:"vote"`
	Message string `json:"message,omitempty" mapstructure:"message"`
}

func VoteResponse(resp *RequestVoteResponse, node Node, vote bool, msg string) {
	resp.Node = node
	resp.Vote = vote
	resp.Message = msg
}

type NodeWithState struct {
	State NodeState `json:"state" mapstructure:"state"`
	Node  ElectNode `json:"node" mapstructure:"node"`
}

// ClusterState represents the state of a cluster, including the nodes that make up the cluster.
type ClusterState struct {
	Nodes map[string]*NodeWithState `json:"nodes" mapstructure:"nodes"`
}

type CommandCode uint

const (
	HeartBeat CommandCode = iota + 1
	RequestVote
	State
)

func (c CommandCode) String() string {
	switch c {
	case HeartBeat:
		return "HeartBeat"
	case RequestVote:
		return "RequestVote"
	case State:
		return "State"
	}
	return "Unknown"
}
