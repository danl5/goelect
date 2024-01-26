package consensus

import (
	"context"
	"fmt"
	"time"

	"github.com/looplab/fsm"
	"golang.org/x/sync/errgroup"

	"github.com/danli001/goelect/internal/config"
	"github.com/danli001/goelect/internal/log"
	"github.com/danli001/goelect/internal/model"
	"github.com/danli001/goelect/internal/rpc"
)

func NewConsensus() (*Consensus, error) {
	return nil, nil
}

type Consensus struct {
	*termCache

	cfg    *config.Config
	logger log.Logger

	node       model.ElectNode
	fsm        *fsm.FSM
	rpcClients map[string]*rpc.Client

	leaderChan    chan struct{}
	followerChan  chan struct{}
	candidateChan chan struct{}
	eventChan     chan model.NodeEvent
}

type termCache struct {
	term    uint64
	voted   bool
	startAt time.Time
}

func (c *Consensus) HeartBeat(args *model.HeartBeatRequest, reply *model.HeartBeatResponse) error {
	c.logger.Debug("receive heartbeat", "from", args.Node)

	switch c.node.State {
	case model.NodeStateLeader:
		reply = &model.HeartBeatResponse{
			Ok:      false,
			Message: "node in leader state",
		}
	case model.NodeStateFollower:
		if c.term < args.Term {
			c.term = args.Term
		}
		c.followerChan <- struct{}{}
	case model.NodeStateCandidate:
		if c.term > args.Term {
			reply = &model.HeartBeatResponse{
				Ok:      false,
				Message: "leader is falling behind",
			}
			return nil
		}
		c.sendEvent(model.EventNewLeader)
	case model.NodeStateDown:
	}
	return nil
}

func (c *Consensus) enterLeader(ctx context.Context, ev *fsm.Event) {
	c.leaderChan = make(chan struct{}, 1)
	go func() {
		err := c.runLeader()
		if err != nil {
			c.logger.Error("failed to enter leader state", "err", err.Error())
			return
		}
	}()
}

func (c *Consensus) runLeader() error {
	tk := time.NewTicker(time.Duration(c.cfg.HeartBeatInterval) * time.Second)
	defer tk.Stop()

	//
	errCount := c.sendHeartBeat()
	//
	if errCount >= c.countVoteNode()/2+1 {
		c.sendEvent(model.EventLeaveLeader)
	}

	for {
		select {
		case <-c.leaderChan:
			c.logger.Info("leave leader", "node", c.node.Address)
			return nil
		case <-tk.C:
			errCount := c.sendHeartBeat()
			//
			if errCount >= c.countVoteNode()/2+1 {
				c.sendEvent(model.EventLeaveLeader)
			}
		}
	}

}

func (c *Consensus) stopLeader(ctx context.Context, ev *fsm.Event) {
	close(c.leaderChan)
}

func (c *Consensus) enterFollower(ctx context.Context, ev *fsm.Event) {
	c.followerChan = make(chan struct{}, 1)
	go func() {
		err := c.runFollower()
		if err != nil {
			c.logger.Error("failed to enter follower state", "err", err.Error())
			return
		}
	}()
}

func (c *Consensus) runFollower() error {
	ts := time.NewTimer(time.Duration(c.cfg.HeartBeatInterval) * time.Second)
	defer ts.Stop()
	for {
		select {
		case _, ok := <-c.followerChan:
			// channel is closed
			if !ok {
				c.logger.Info("leave follower state", "node", c.node.Address)
				return nil
			}
			if !ts.Stop() {
				select {
				case <-ts.C:
				default:
				}
			}
			ts.Reset(time.Duration(c.cfg.HeartBeatInterval) * time.Second)
		case <-ts.C:
			c.sendEvent(model.EventHeartbeatTimeout)
		}
	}
}

func (c *Consensus) stopFollower(ctx context.Context, ev *fsm.Event) {
	close(c.followerChan)
}

func (c *Consensus) enterCandidate(ctx context.Context, ev *fsm.Event) {
	c.candidateChan = make(chan struct{}, 1)
	go func() {
		err := c.runCandidate()
		if err != nil {
			c.logger.Error("failed to enter candidate state", "err", err.Error())
			return
		}
	}()
}

func (c *Consensus) runCandidate() error {
	return nil
}

func (c *Consensus) stopCandidate(ctx context.Context, ev *fsm.Event) {
	close(c.candidateChan)
}

func (c *Consensus) sendEvent(ev model.NodeEvent) {
	c.eventChan <- ev
}

func (c *Consensus) sendHeartBeat() (errorCount int) {
	g := errgroup.Group{}

	for nodeIP, clt := range c.rpcClients {
		nodeIP, clt := nodeIP, clt
		g.Go(func() error {
			resp := &model.HeartBeatResponse{}
			err := clt.Call("HeartBeat", &model.HeartBeatRequest{Node: c.node.Address, Term: c.term}, resp)
			if err != nil {
				errorCount += 1
				return err
			}
			if !resp.Ok {
				errorCount += 1
				return fmt.Errorf("heartbeat, peer response not ok, message %s", resp.Message)
			}

			c.logger.Info("send heartbeat to peer", "peer", nodeIP)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		c.logger.Error("leader, heartbeat error", "error", err.Error())
	}

	return
}

func (c *Consensus) sendRequestVote() int {
	g := errgroup.Group{}
	//for nodeIP, clt := range c.rpcClients {
	//	//nodeIP, clt := nodeIP, clt
	//	g.Go(func() error {
	//		resp
	//	})
	//
	//}
	return 0
}

func (c *Consensus) countVoteNode() (count int) {
	for _, n := range c.cfg.Peers {
		if n.NoVote {
			continue
		}
		count += 1
	}
	return
}

func (c *Consensus) initializeFsm() {
	c.fsm = fsm.NewFSM(
		model.NodeStateFollower.String(),
		fsm.Events{
			{
				Name: model.EventHeartbeatTimeout.String(),
				Src:  []string{model.NodeStateFollower.String()},
				Dst:  model.NodeStateCandidate.String(),
			},
			{
				Name: model.EventLeaveLeader.String(),
				Src:  []string{model.NodeStateLeader.String()},
				Dst:  model.NodeStateFollower.String(),
			},
			{
				Name: model.EventMajorityVotes.String(),
				Src:  []string{model.NodeStateCandidate.String()},
				Dst:  model.NodeStateLeader.String(),
			},
			{
				Name: model.EventNewLeader.String(),
				Src:  []string{model.NodeStateCandidate.String()},
				Dst:  model.NodeStateFollower.String(),
			},
			{
				Name: model.EventDown.String(),
				Src: []string{
					model.NodeStateLeader.String(),
					model.NodeStateFollower.String(),
					model.NodeStateCandidate.String(),
				},
				Dst: model.NodeStateDown.String(),
			},
		},
		fsm.Callbacks{
			"enter_" + model.NodeStateLeader.String(): func(ctx context.Context, ev *fsm.Event) {
				c.enterLeader(ctx, ev)
			},
			"leave_" + model.NodeStateLeader.String(): func(ctx context.Context, ev *fsm.Event) {
				c.stopLeader(ctx, ev)
			},
			"enter_" + model.NodeStateFollower.String(): func(ctx context.Context, ev *fsm.Event) {
				c.enterFollower(ctx, ev)
			},
			"leave_" + model.NodeStateFollower.String(): func(ctx context.Context, ev *fsm.Event) {
				c.stopFollower(ctx, ev)
			},
			"enter_" + model.NodeStateCandidate.String(): func(ctx context.Context, ev *fsm.Event) {
				c.enterCandidate(ctx, ev)
			},
			"leave_" + model.NodeStateCandidate.String(): func(ctx context.Context, ev *fsm.Event) {
				c.stopCandidate(ctx, ev)
			},
		},
	)
}
