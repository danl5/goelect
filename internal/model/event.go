package model

type NodeEvent string

const (
	EventHeartbeatTimeout NodeEvent = "heartbeat_timeout"
	EventLeaveLeader      NodeEvent = "leave_leader"
	EventNewLeader        NodeEvent = "new_leader"
	EventMajorityVotes    NodeEvent = "majority_votes"
	EventDown             NodeEvent = "down"
)

func (n NodeEvent) String() string {
	return string(n)
}
