package common

type VoteMessage string

const (
	VoteOk          VoteMessage = `ok`
	VoteTermExpired VoteMessage = `term has expired`
	VoteLeaderExist VoteMessage = `leader exist`
	VoteHaveVoted   VoteMessage = `have voted`
	VoteNoVoteNode  VoteMessage = `no vote node`
)

func (v VoteMessage) String() string {
	return string(v)
}

type HeartbeatMessage string

const (
	HeartbeatOk         HeartbeatMessage = `ok`
	HeartbeatLeaderNode HeartbeatMessage = `node in leader state`
	HeartbeatExpired    HeartbeatMessage = `term has expired`
)

func (h HeartbeatMessage) String() string {
	return string(h)
}
