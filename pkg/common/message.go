package common

// VoteResponseMessage is the message used to indicate the reason in the response to the voting request
type VoteResponseMessage string

const (
	// VoteOk represents a vote in agreement
	VoteOk VoteResponseMessage = `ok`
	// VoteTermExpired represents the term has expired
	VoteTermExpired VoteResponseMessage = `term has expired`
	// VoteLeaderExist represents that a leader already exists
	VoteLeaderExist VoteResponseMessage = `leader exist`
	// VoteHaveVoted represents that a vote has already been cast in this term
	VoteHaveVoted VoteResponseMessage = `have voted`
	// VoteNoVoteNode represents a node with a non-voting role
	VoteNoVoteNode VoteResponseMessage = `no vote node`
)

func (v VoteResponseMessage) String() string {
	return string(v)
}

// HeartbeatMessage is the message used to indicate the reason in the response to the heartbeat request
type HeartbeatMessage string

const (
	// HeartbeatOk represents that the heartbeat is normal
	HeartbeatOk HeartbeatMessage = `ok`
	// HeartbeatExpired represents that the term has fallen behind
	HeartbeatExpired HeartbeatMessage = `term has expired`
)

func (h HeartbeatMessage) String() string {
	return string(h)
}
