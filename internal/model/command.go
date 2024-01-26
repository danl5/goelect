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

type RequestVoteRequest struct {
	NodeId string
	Term   uint64
}

type RequestVoteResponse struct {
	NodeId  string
	Vote    bool
	Message string
}
