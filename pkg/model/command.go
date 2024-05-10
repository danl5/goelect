package model

// HeartBeatRequest is the heartbeat request
type HeartBeatRequest struct {
	NodeId string `json:"node_id"`
	Term   uint64 `json:"term"`
}

// HeartBeatResponse is the heartbeat response
type HeartBeatResponse struct {
	Ok      bool   `json:"ok,omitempty"`
	Message string `json:"message,omitempty"`
}

func HBResponse(resp *HeartBeatResponse, ok bool, msg string) {
	resp.Ok = ok
	resp.Message = msg
}

// RequestVoteRequest is the vote request
type RequestVoteRequest struct {
	NodeId   string `json:"node_id"`
	Term     uint64 `json:"term"`
	NodeAddr string `json:"node_addr"`
}

// RequestVoteResponse is the vote response
type RequestVoteResponse struct {
	Node    Node   `json:"node"`
	Vote    bool   `json:"vote"`
	Message string `json:"message,omitempty"`
}

func VoteResponse(resp *RequestVoteResponse, node Node, vote bool, msg string) {
	resp.Node = node
	resp.Vote = vote
	resp.Message = msg
}

// ClusterState represents the state of a cluster, including the nodes that make up the cluster.
type ClusterState struct {
	Nodes map[string]*ElectNode `json:"nodes"`
}
