package common

import (
	"github.com/danl5/goelect/internal/model"
)

type RpcHandler interface {
	// HeartBeat handles heartbeat request from leader
	HeartBeat(args *model.HeartBeatRequest, reply *model.HeartBeatResponse) error

	// RequestVote handle vote request from peer nodes
	RequestVote(args *model.RequestVoteRequest, reply *model.RequestVoteResponse) error

	// State return current node state
	State(args struct{}, reply *model.ElectNode) error

	// Ping handles ping request from peer node
	Ping(args struct{}, reply *string) error
}
