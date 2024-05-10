package model

// NodeEvent represents the related events in the entire lifecycle of the node,
// used to drive the node Finite State Machine (FSM)
type NodeEvent string

const (
	// EventHeartbeatTimeout represents the node heartbeat timeout
	EventHeartbeatTimeout NodeEvent = "heartbeat_timeout"
	// EventLeaveLeader represents the node leaving the leader state
	EventLeaveLeader NodeEvent = "leave_leader"
	// EventNewLeader represents the node discovering a new leader
	EventNewLeader NodeEvent = "new_leader"
	// EventNewTerm represents the node discovering a new term
	EventNewTerm NodeEvent = "new_term"
	// EventMajorityVotes represents the node receiving the majority of votes
	EventMajorityVotes NodeEvent = "majority_votes"
	// EventDown represents the node going offline
	EventDown NodeEvent = "down"
)

func (n NodeEvent) String() string {
	return string(n)
}
