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
	// initialize the node FSM
	c.initializeFsm()
	return c, nil
}

type Consensus struct {
	// termCache holds the current term state
	*termCache

	// cfg is the configuration for the consensus
	cfg *config.Config
	// logger
	logger log.Logger

	// node is the elect node of the model
	node model.ElectNode
	// fsm is the finite state machine of the elect node
	fsm *fsm.FSM
	// rpcClients is used to communicate with other nodes
	rpcClients *rpcClients

	// eventChan is used to transmit node event
	eventChan chan model.NodeEvent
	// nodeStateChan is used to transmit node state
	nodeStateChan chan model.StateTransition
	// leaderChan is used to send leader event
	leaderChan chan struct{}
	// followerChan is used to send follower event
	followerChan chan struct{}
	// candidateChan is used to send candidate event
	candidateChan chan struct{}
	// shutdownChan is used to send shutdown event
	shutdownChan chan struct{}
}

// Run starts the consensus
// Returns a channel of state transitions and an error
func (c *Consensus) Run() (<-chan model.StateTransition, error) {
	// iterate through the list of peers
	for _, nodeCfg := range c.cfg.Peers {
		if nodeCfg.ID == c.node.ID {
			// skip self
			continue
		}
		// create a new RPC client for every peer
		clt, err := rpc.NewRpcClient(nodeCfg.Address, c.cfg.ConnectTimeout, c.logger, func(client *nrpc.Client) error {
			var reply string
			return client.Call("Consensus.Ping", nil, &reply)
		})
		if err != nil {
			c.logger.Error("can not connect to peer", "peer", nodeCfg.Address, "error", err.Error())
			return nil, err
		}
		c.rpcClients.put(nodeCfg.Address, clt)
	}
	// run the event handler
	c.runEventHandler()

	// enter the default follower state
	c.enterFollower(context.Background(), &fsm.Event{Dst: model.NodeStateFollower.String()})

	c.logger.Debug("start consensus")
	return c.nodeStateChan, nil
}

// HeartBeat handles heartbeat request from peer node
func (c *Consensus) HeartBeat(args *model.HeartBeatRequest, reply *model.HeartBeatResponse) error {
	c.logger.Debug("receive heartbeat", "from", args.NodeId)

	if c.term > args.Term {
		// term in the request is behind this node
		model.HBResponse(reply, false, common.HeartbeatExpired.String())
		return nil
	}

	// update term of this node
	c.setTerm(args.Term)

	switch c.node.State {
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

func (c *Consensus) Visualize() string {
	return fsm.Visualize(c.fsm)
}

// RequestVote handle vote request from peer node
func (c *Consensus) RequestVote(args *model.RequestVoteRequest, reply *model.RequestVoteResponse) error {
	c.logger.Info("receive vote request", "from", args.NodeAddr, "term", args.Term, "current term", c.term)
	// return when novote is true
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
func (c *Consensus) Ping(args struct{}, reply *string) error {
	*reply = "pong"
	return nil
}

// CurrentState return current node state
func (c *Consensus) CurrentState() model.NodeState {
	return c.node.State
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
	tk := time.NewTicker(c.cfg.HeartBeatInterval)
	defer tk.Stop()

	for {
		var errCount int
		// send heartbeat to followers
		c.sendHeartBeat(&errCount)
		// leaves leader state if the number of errors is more than half
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
	// set the heartbeat timeout to twice the heartbeat interval
	heartbeatTimeout := c.cfg.HeartBeatInterval * 2
	ts := time.NewTimer(heartbeatTimeout)
	defer ts.Stop()
	for {
		select {
		case _, ok := <-c.followerChan:
			if !ok {
				// channel is closed
				c.logger.Info("leave follower state", "node", c.node.Address)
				return nil
			}
			// reset the timer
			if !ts.Stop() {
				select {
				case <-ts.C:
				default:
				}
			}
			ts.Reset(heartbeatTimeout)
		case <-ts.C:
			// heartbeat timeout
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
		c.logger.Info("novote node, not participate in the election", "node", c.node.Address)
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
	ts := time.NewTicker(c.cfg.ElectTimeout)
	defer ts.Stop()

	electDelay := func() {
		delayDuration := time.Duration(rand.Int63n(int64(c.cfg.ElectTimeout)))
		time.Sleep(delayDuration)
	}
	for {
		// randomized delay to reduce the probability of all nodes
		// initiating voting requests at the same time
		electDelay()

		// update term
		c.incrementByOne()
		// vote for self
		voteCount := 1
		voteChan := make(chan model.Node)
		go func() {
			// send vote request to all peer nodes
			err := c.sendRequestVote(voteChan)
			if err != nil {
				c.logger.Error("failed to send vote request", "error", err.Error())
				return
			}
		}()

		for {
			// become a leader when receive more than half of the votes
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
			// this term timeout, will start the next
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
			// check if the event is legal
			ok := c.fsm.Can(ev.String())
			if !ok {
				c.logger.Error("wrong event", "current state", c.fsm.Current(), "event", ev.String())
				// faulty state migration is unacceptable
				panic("unrecoverable error: wrong state transition")
			}

			err := c.fsm.Event(context.TODO(), ev.String())
			if err != nil {
				c.logger.Error("error state transition", "current state", c.fsm.Current(), "event", ev.String())
				// faulty state migration is unacceptable
				panic("unrecoverable error: wrong state transition")
			}
		}
	}()
}

func (c *Consensus) sendHeartBeat(errorCount *int) {
	g := errgroup.Group{}
	for rpcClient := range c.rpcClients.iterate() {
		nodeAddr, clt := rpcClient.address, rpcClient.client
		if nodeAddr == c.node.Address {
			// skip self
			continue
		}
		g.Go(func() error {
			resp := &model.HeartBeatResponse{}
			// send heartbeat request to peer
			err := clt.Call("Consensus.HeartBeat", &model.HeartBeatRequest{NodeId: c.node.ID, Term: c.term}, resp)
			if err != nil {
				// error count
				*errorCount += 1
				c.logger.Error("failed to send heartbeat", "peer", nodeAddr, "error", err.Error())
				return fmt.Errorf("heartbeat, failed to send heartbeat, peer %s, error %s", nodeAddr, err.Error())
			}
			if !resp.Ok {
				// error count
				*errorCount += 1
				return fmt.Errorf("heartbeat, peer %s response not ok, message %s", nodeAddr, resp.Message)
			}

			c.logger.Debug("send heartbeat to peer", "peer", nodeAddr)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		c.logger.Error("leader, heartbeat error", "error", err.Error())
	}
}

func (c *Consensus) sendRequestVote(voteChan chan model.Node) error {
	g := errgroup.Group{}
	defer close(voteChan)

	for rpcClient := range c.rpcClients.iterate() {
		nodeAddr, clt := rpcClient.address, rpcClient.client
		if nodeAddr == c.node.Address {
			// skip self
			continue
		}

		g.Go(func() error {
			resp := &model.RequestVoteResponse{}
			// send vote request
			err := clt.Call("Consensus.RequestVote", &model.RequestVoteRequest{
				NodeId:   c.node.ID,
				Term:     c.term,
				NodeAddr: c.node.Address,
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
				// receive a valid vote from peer node
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
			// skip novote node
			continue
		}
		count += 1
	}
	return
}

// initializeFsm initializes the state machine of an elect node
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
			"enter_" + model.NodeStateLeader.String():    c.enterLeader,
			"leave_" + model.NodeStateLeader.String():    c.leaveLeader,
			"enter_" + model.NodeStateFollower.String():  c.enterFollower,
			"leave_" + model.NodeStateFollower.String():  c.leaveFollower,
			"enter_" + model.NodeStateCandidate.String(): c.enterCandidate,
			"leave_" + model.NodeStateCandidate.String(): c.leaveCandidate,
			"enter_" + model.NodeStateDown.String():      c.enterShutdown,
			"leave_" + model.NodeStateDown.String():      c.leaveShutdown,
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
	// when term is changed, set voted and voteFor to empty value
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
