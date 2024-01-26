package model

type CommandCode uint

const (
	CommandHeartBeat   CommandCode = iota
	CommandRequestVote CommandCode = iota
)

const (
	MethodHeartBeat = ``
)

type HeartBeatRequest struct {
	Node string
	Term uint64
}

type HeartBeatResponse struct {
	Ok      bool
	Message string
}

func HBResponse(resp *HeartBeatResponse, ok bool, msg string) {
	resp.Ok = ok
	resp.Message = msg
}

type RequestVoteRequest struct {
	NodeId string
	Term   uint64
	Node   string
}

type RequestVoteResponse struct {
	Node    Node
	Vote    bool
	Message string
}

func VoteResponse(resp *RequestVoteResponse, node Node, vote bool, msg string) {
	resp.Node = node
	resp.Vote = vote
	resp.Message = msg
}
