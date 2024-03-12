package common

// VoteMessage is the message used to indicate the reason in the response to the voting request
type VoteMessage string

const (
	// VoteOk represents a vote in agreement
	VoteOk VoteMessage = `ok`
	// VoteTermExpired represents the term has expired
	VoteTermExpired VoteMessage = `term has expired`
	// VoteLeaderExist represents that a leader already exists
	VoteLeaderExist VoteMessage = `leader exist`
	// VoteHaveVoted represents that a vote has already been cast in this term
	VoteHaveVoted VoteMessage = `have voted`
	// VoteNoVoteNode represents a node with a non-voting role
	VoteNoVoteNode VoteMessage = `no vote node`
)

func (v VoteMessage) String() string {
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
