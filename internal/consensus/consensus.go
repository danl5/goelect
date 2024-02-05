package consensus

import (
	"context"
	"fmt"
	"math/rand"
	nrpc "net/rpc"
	"sync"
	"time"

	"github.com/looplab/fsm"
	"golang.org/x/sync/errgroup"

	"github.com/danli001/goelect/internal/common"
	"github.com/danli001/goelect/internal/config"
	"github.com/danli001/goelect/internal/log"
	"github.com/danli001/goelect/internal/model"
	"github.com/danli001/goelect/internal/rpc"
)

const (
	// set the election timeout to 200 milliseconds
	electTimeout = 200 * time.Millisecond

	heartBeatInterval = 150 * time.Millisecond
)

func NewConsensus(cfg *config.Config, logger log.Logger, node model.ElectNode) (*Consensus, error) {
	c := &Consensus{
		cfg:           cfg,
		logger:        logger,
		node:          node,
		termCache:     &termCache{},
		rpcClients:    &rpcClients{},
		leaderChan:    make(chan struct{}, 1),
		followerChan:  make(chan struct{}, 1),
		candidateChan: make(chan struct{}, 1),
		shutdownChan:  make(chan struct{}, 1),
		eventChan:     make(chan model.NodeEvent, 1),
		nodeStateChan: make(chan model.StateTransition, 1),
	}
	return c, nil
}

type Consensus struct {
	*termCache

	cfg    *config.Config
	logger log.Logger

	node       model.ElectNode
	fsm        *fsm.FSM
	rpcClients *rpcClients

	eventChan     chan model.NodeEvent
	nodeStateChan chan model.StateTransition
	leaderChan    chan struct{}
	followerChan  chan struct{}
	candidateChan chan struct{}
	shutdownChan  chan struct{}
}

// Run starts the consensus algorithm
// Returns a channel of state transitions and an error
func (c *Consensus) Run() (<-chan model.StateTransition, error) {
	// iterate through the list of peers
	for _, nodeCfg := range c.cfg.Peers {
		if nodeCfg.ID == c.node.ID {
			// skip self
			continue
		}
		// create a new RPC client for every peer
		clt, err := rpc.NewRpcClient(nodeCfg.Address, c.logger, func(client *nrpc.Client) error {
			var reply string
			return client.Call("Consensus.Ping", nil, &reply)
		})
		if err != nil {
			c.logger.Error("can not connect to peer", "peer", nodeCfg.Address, "error", err.Error())
			return nil, err
		}
		c.rpcClients.put(nodeCfg.Address, clt)
	}
	// initialize the node FSM
	c.initializeFsm()
	// run the event handler
	c.runEventHandler()

	// enter the default follower state
	c.enterFollower(context.Background(), &fsm.Event{Dst: model.NodeStateFollower.String()})

	c.logger.Debug("start consensus")
	return c.nodeStateChan, nil
}

func (c *Consensus) HeartBeat(args *model.HeartBeatRequest, reply *model.HeartBeatResponse) error {
	c.logger.Debug("receive heartbeat", "from", args.Node)

	switch c.node.State {
	case model.NodeStateLeader:
		model.HBResponse(reply, false, common.HeartbeatLeaderNode.String())
	case model.NodeStateFollower:
		if c.term < args.Term {
			c.setTerm(args.Term)
		}
		c.followerChan <- struct{}{}
		model.HBResponse(reply, true, common.HeartbeatOk.String())
	case model.NodeStateCandidate:
		if c.term > args.Term {
			model.HBResponse(reply, false, common.HeartbeatExpired.String())
			return nil
		}
		c.sendEvent(model.EventNewLeader)
	case model.NodeStateDown:
	}
	return nil
}

func (c *Consensus) RequestVote(args *model.RequestVoteRequest, reply *model.RequestVoteResponse) error {
	c.logger.Info("receive vote request", "from", args.Node, "term", args.Term, "current term", c.term)
	//
	if c.node.NoVote {
		model.VoteResponse(reply, c.node.Node, false, common.VoteNoVoteNode.String())
		return nil
	}

	switch c.node.State {
	case model.NodeStateLeader:
		if args.Term <= c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteLeaderExist.String())
			return nil
		}
		//
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
		//
		c.sendEvent(model.EventNewTerm)
	case model.NodeStateDown:
	}

	c.setTerm(args.Term)
	c.vote(args.Term, args.Node)
	c.logger.Info("vote for", "node", args.Node, "term", args.Term)
	model.VoteResponse(reply, c.node.Node, true, common.VoteOk.String())
	return nil
}

func (c *Consensus) Ping(args struct{}, reply *string) error {
	*reply = "pong"
	return nil
}

func (c *Consensus) enterLeader(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become leader")
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
	c.node.State = model.NodeStateLeader
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
	tk := time.NewTicker(heartBeatInterval)
	defer tk.Stop()

	for {
		var errCount int
		//
		c.sendHeartBeat(&errCount)
		//
		if errCount >= c.countVoteNode()/2+1 {
			c.sendEvent(model.EventLeaveLeader)
		}

		select {
		case <-c.leaderChan:
			c.logger.Info("leave leader", "node", c.node.Address)
			return nil
		case <-tk.C:
		}
	}
}

func (c *Consensus) leaveLeader(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("leave leader")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.leaderChan)
}

func (c *Consensus) enterFollower(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become follower")
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
	c.node.State = model.NodeStateFollower
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
	//
	heartbeatTimeout := heartBeatInterval * 2
	ts := time.NewTimer(heartbeatTimeout)
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
			ts.Reset(heartbeatTimeout)
		case <-ts.C:
			c.sendEvent(model.EventHeartbeatTimeout)
		}
	}
}

func (c *Consensus) leaveFollower(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("leave follower")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.followerChan)
}

func (c *Consensus) enterCandidate(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become candidate")
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
	c.node.State = model.NodeStateCandidate
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
	if c.node.NoVote {
		c.logger.Info("no vote node, not participate in the election", "node", c.node.Address)
		return nil
	}

	err := c.tryToBecomeLeader()
	if err != nil {
		c.logger.Error("failed to make the try to become leader", "err", err.Error())
		return err
	}

	return nil
}

func (c *Consensus) tryToBecomeLeader() error {
	ts := time.NewTicker(electTimeout)
	defer ts.Stop()

	electDelay := func() {
		delayDuration := time.Duration(rand.Int63n(int64(electTimeout)))
		time.Sleep(delayDuration)
	}
	for {
		electDelay()
		c.incrementByOne()
		voteCount := 1
		voteChan := make(chan model.Node)
		go func() {
			err := c.sendRequestVote(voteChan)
			if err != nil {
				c.logger.Error("failed to send vote request", "error", err.Error())
				return
			}
		}()

		for {
			if voteCount >= c.countVoteNode()/2+1 {
				c.logger.Info("received more than half of the votes, try to become leader")
				c.sendEvent(model.EventMajorityVotes)
				return nil
			}

			select {
			case node, ok := <-voteChan:
				if !ok {
					c.logger.Info("vote end", "term", int(c.term), "count", voteCount)
					goto nextLoop
				}
				c.logger.Info("receive vote", "peer", node.Address)
				voteCount += 1
				if voteCount >= c.countVoteNode()/2+1 {
					c.logger.Info("received more than half of the votes, become leader")
					c.sendEvent(model.EventMajorityVotes)
					return nil
				}
			case <-c.candidateChan:
				c.logger.Info("stop receive vote response")
				return nil
			}
		}
	nextLoop:
		select {
		case <-ts.C:
		case <-c.candidateChan:
			c.logger.Info("leave candidate state")
			return nil
		}
	}
}

func (c *Consensus) leaveCandidate(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("leave candidate")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.candidateChan)
}

func (c *Consensus) enterShutdown(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become shutdown")
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
}

func (c *Consensus) leaveShutdown(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("leave shutdown")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.shutdownChan)
}

func (c *Consensus) sendEvent(ev model.NodeEvent) {
	c.eventChan <- ev
	c.logger.Debug("node event", "event", ev.String())
}

func (c *Consensus) runEventHandler() {
	go func() {
		for ev := range c.eventChan {
			ok := c.fsm.Can(ev.String())
			if !ok {
				c.logger.Fatal("wrong event", "current state", c.fsm.Current(), "event", ev.String())
			}

			err := c.fsm.Event(context.TODO(), ev.String())
			if err != nil {
				c.logger.Fatal("error state transition", "current state", c.fsm.Current(), "event", ev.String())
			}
		}
	}()
}

func (c *Consensus) sendHeartBeat(errorCount *int) {
	g := errgroup.Group{}
	for rpcClient := range c.rpcClients.iterate() {
		nodeAddr, clt := rpcClient.address, rpcClient.client
		if nodeAddr == c.node.Address {
			continue
		}
		g.Go(func() error {
			resp := &model.HeartBeatResponse{}
			err := clt.Call("Consensus.HeartBeat", &model.HeartBeatRequest{Node: c.node.Address, Term: c.term}, resp)
			if err != nil {
				*errorCount += 1
				c.logger.Error("failed to send heartbeat", "peer", nodeAddr, "error", err.Error())
				return fmt.Errorf("heartbeat, failed to send heartbeat, peer %s, error %s", nodeAddr, err.Error())
			}
			if !resp.Ok {
				*errorCount += 1
				return fmt.Errorf("heartbeat, peer %s response not ok, message %s", nodeAddr, resp.Message)
			}

			c.logger.Info("send heartbeat to peer", "peer", nodeAddr)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		c.logger.Error("leader, heartbeat error", "error", err.Error())
	}
	return
}

func (c *Consensus) sendRequestVote(voteChan chan model.Node) error {
	g := errgroup.Group{}
	defer close(voteChan)

	for rpcClient := range c.rpcClients.iterate() {
		nodeAddr, clt := rpcClient.address, rpcClient.client
		if nodeAddr == c.node.Address {
			continue
		}

		g.Go(func() error {
			resp := &model.RequestVoteResponse{}
			err := clt.Call("Consensus.RequestVote", &model.RequestVoteRequest{
				NodeId: c.node.Address,
				Term:   c.term,
				Node:   c.node.Address,
			}, &resp)
			if err != nil {
				c.logger.Error("failed to get vote", "peer", nodeAddr)
				return fmt.Errorf("failed to get vote, peer %s, err: %s", nodeAddr, err.Error())
			}
			if !resp.Vote {
				c.logger.Info("peer disagrees with voting for you", "peer", nodeAddr, "message", resp.Message)
				return nil
			}

			select {
			case <-c.candidateChan:
				return nil
			case voteChan <- resp.Node:
			}
			c.logger.Info("get voting", "peer", nodeAddr)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		c.logger.Error("candidate, request voting error", "error", err.Error())
		return err
	}

	return nil
}

func (c *Consensus) sendNodeStateTransition(state, srcState model.NodeState, transType model.TransitionType) {
	c.nodeStateChan <- model.StateTransition{
		State:    state,
		SrcState: srcState,
		Type:     transType,
	}
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
				Name: model.EventNewTerm.String(),
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
				c.leaveLeader(ctx, ev)
			},
			"enter_" + model.NodeStateFollower.String(): func(ctx context.Context, ev *fsm.Event) {
				c.enterFollower(ctx, ev)
			},
			"leave_" + model.NodeStateFollower.String(): func(ctx context.Context, ev *fsm.Event) {
				c.leaveFollower(ctx, ev)
			},
			"enter_" + model.NodeStateCandidate.String(): func(ctx context.Context, ev *fsm.Event) {
				c.enterCandidate(ctx, ev)
			},
			"leave_" + model.NodeStateCandidate.String(): func(ctx context.Context, ev *fsm.Event) {
				c.leaveCandidate(ctx, ev)
			},
			"enter_" + model.NodeStateDown.String(): func(ctx context.Context, ev *fsm.Event) {
				c.enterShutdown(ctx, ev)
			},
			"leave_" + model.NodeStateDown.String(): func(ctx context.Context, ev *fsm.Event) {
				c.leaveShutdown(ctx, ev)
			},
		},
	)
}

type termCache struct {
	term    uint64
	voted   bool
	voteFor string
}

func (t *termCache) setTerm(term uint64) bool {
	if term < t.term {
		return false
	} else if term == t.term {
		return true
	}
	t.term = term
	t.voted = false
	t.voteFor = ""
	return true
}

func (t *termCache) vote(term uint64, node string) bool {
	if term != t.term {
		return false
	}
	t.voted = true
	t.voteFor = node
	return true
}

func (t *termCache) incrementByOne() {
	t.term += 1
	t.voted = false
	t.voteFor = ""
}

type rpcClients struct {
	clients map[string]*rpc.Client
	m       sync.RWMutex
}

func (r *rpcClients) get(addr string) *rpc.Client {
	r.m.RLock()
	defer r.m.RUnlock()

	if clt, ok := r.clients[addr]; ok {
		return clt
	}

	return nil
}

func (r *rpcClients) put(addr string, clt *rpc.Client) {
	r.m.Lock()
	defer r.m.Unlock()

	if r.clients == nil {
		r.clients = map[string]*rpc.Client{}
	}
	r.clients[addr] = clt
}

func (r *rpcClients) iterate() <-chan *rpcClient {
	c := make(chan *rpcClient)
	go func() {
		r.m.RLock()
		defer r.m.RUnlock()

		for addr, clt := range r.clients {
			c <- &rpcClient{
				address: addr,
				client:  clt,
			}
		}
		close(c)
	}()
	return c
}

type rpcClient struct {
	address string
	client  *rpc.Client
}
