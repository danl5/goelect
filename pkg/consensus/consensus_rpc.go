package consensus

import (
	"github.com/danl5/goelect/pkg/common"
	"github.com/danl5/goelect/pkg/config"
	"github.com/danl5/goelect/pkg/log"
	"github.com/danl5/goelect/pkg/model"
)

func NewConsensusRpcHandler(cfg *config.Config, logger log.Logger, node model.ElectNode) (*RpcHandler, error) {
	c, err := NewConsensus(cfg, logger, node)
	if err != nil {
		logger.Error("failed to create a Consensus instance", "err", err.Error())
		return nil, err
	}
	return &RpcHandler{c}, nil
}

type RpcHandler struct {
	*Consensus
}

// Run starts the consensus
// Returns a channel of state transitions and an error
func (c *RpcHandler) Run() (<-chan model.StateTransition, error) {
	return c.Consensus.Run()
}

// HeartBeat handles heartbeat request from peer node
func (c *RpcHandler) HeartBeat(args *model.HeartBeatRequest, reply *model.HeartBeatResponse) error {
	c.logger.Debug("receive heartbeat", "from", args.NodeId)

	if c.term > args.Term {
		// term in the request is behind this node
		model.HBResponse(reply, false, common.HeartbeatExpired.String())
		return nil
	}

	// update term of this node
	c.setTerm(args.Term)

	switch model.NodeState(c.fsm.Current()) {
	case model.NodeStateLeader:
		// leave leader state
		c.sendEvent(model.EventLeaveLeader)
	case model.NodeStateFollower:
		// send heartbeat to handler
		c.followerChan <- struct{}{}
	case model.NodeStateCandidate:
		// receive a new leader
		c.sendEvent(model.EventNewLeader)
	case model.NodeStateDown:
	}

	model.HBResponse(reply, true, common.HeartbeatOk.String())
	return nil
}

// RequestVote handle vote request from peer node
func (c *RpcHandler) RequestVote(args *model.RequestVoteRequest, reply *model.RequestVoteResponse) error {
	c.logger.Info("receive vote request", "from", args.NodeAddr, "term", args.Term, "current term", c.term)
	// return when no-vote is true
	if c.node.NoVote {
		model.VoteResponse(reply, c.node.Node, false, common.VoteNoVoteNode.String())
		return nil
	}

	switch model.NodeState(c.fsm.Current()) {
	case model.NodeStateLeader:
		if args.Term <= c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteLeaderExist.String())
			return nil
		}
		// term in the request is newer, leaves leader state
		c.sendEvent(model.EventLeaveLeader)
	case model.NodeStateFollower:
		if args.Term < c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteTermExpired.String())
			return nil
		}
	case model.NodeStateCandidate:
		if args.Term <= c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteHaveVoted.String())
			return nil
		}
		// term in the request is newer, vote and switch to follower state
		c.sendEvent(model.EventNewTerm)
	case model.NodeStateDown:
	}

	// update term cache
	c.setTerm(args.Term)
	c.vote(args.Term, args.NodeId)

	c.logger.Info("vote for", "node", args.NodeId, "term", args.Term)
	model.VoteResponse(reply, c.node.Node, true, common.VoteOk.String())
	return nil
}

// Ping handles ping request from peer node
func (c *RpcHandler) Ping(_ struct{}, reply *string) error {
	*reply = "pong"
	return nil
}

// State return current node state
func (c *RpcHandler) State(_ struct{}, reply *model.NodeWithState) error {
	*reply = c.CurrentState()
	return nil
}
