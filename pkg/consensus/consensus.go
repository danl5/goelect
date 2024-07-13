package consensus

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/looplab/fsm"
	"golang.org/x/sync/errgroup"

	"github.com/danl5/goelect/pkg/common"
	"github.com/danl5/goelect/pkg/config"
	"github.com/danl5/goelect/pkg/model"
)

func NewConsensus(
	node model.ElectNode,
	trans model.Transport,
	transConfig model.TransportConfig,
	cfg *config.Config,
	logger *slog.Logger) (*Consensus, error) {
	if err := node.Validate(); err != nil {
		return nil, err
	}

	if logger == nil {
		return nil, fmt.Errorf("new consensus, logger is nil")
	}

	c := &Consensus{
		cfg:             cfg,
		logger:          logger.With("component", "consensus"),
		node:            node,
		termCache:       &termCache{},
		transport:       trans,
		transportConfig: transConfig,
		leaderChan:      make(chan struct{}, 1),
		followerChan:    make(chan struct{}, 1),
		candidateChan:   make(chan struct{}, 1),
		shutdownChan:    make(chan struct{}, 1),
		eventChan:       make(chan model.NodeEvent, 1),
		nodeStateChan:   make(chan model.StateTransition, 1),
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
	logger *slog.Logger

	// node is the elect node of the model
	node model.ElectNode
	// fsm is the finite state machine of the elect node
	fsm *fsm.FSM
	// transport is the transport layer
	transport model.Transport
	// transportConfig is the transport configuration
	transportConfig model.TransportConfig

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

	// preEventState holds the node state before an event is processed.
	// This allows for comparison and analysis of state changes after the event handling.
	preEventState model.NodeState

	// inLeaderState indicates whether the current node is in the leader state.
	inLeaderState bool
	// inFollowerState indicates whether the current node is in the follower state.
	inFollowerState bool
	// inCandidateState indicates whether the current node is in the candidate state.
	inCandidateState bool
	// inDownState indicates whether the current node is in a down state.
	inDownState bool
}

// Run starts the consensus
// Returns a channel of state transitions and an error
func (c *Consensus) Run() (<-chan model.StateTransition, error) {
	// initialize the transport layer
	err := c.initializeTransport()
	if err != nil {
		return nil, err
	}

	// run the event handler
	c.runEventHandler()

	// enter the default follower state
	c.enterFollower(context.Background(), &fsm.Event{Dst: model.NodeStateFollower.String()})

	c.logger.Info("consensus started")
	return c.nodeStateChan, nil
}

func (c *Consensus) HandleRequest(request *model.Request, response *model.Response) error {
	response.Header = c.buildHeaders()

	switch request.CommandCode {
	case model.HeartBeat:
		c.logger.Debug("receive heartbeat request", "peer", request.Header.Node.ID)
		hbRequest := &model.HeartBeatRequest{}
		err := c.transport.Decode(request.Command, hbRequest)
		if err != nil {
			c.logger.Error("failed to decode heartbeat request", "error", err.Error())
			response.Error = model.ErrorBadCommand
			return model.ErrorBadCommand
		}
		resp := &model.HeartBeatResponse{}
		err = c.HeartBeat(hbRequest, resp)
		if err != nil {
			response.Error = err
			return err
		}
		response.CommandResponse = resp
		return nil
	case model.RequestVote:
		c.logger.Info("receive vote request", "peer", request.Header.Node.ID)
		voteRequest := &model.RequestVoteRequest{}
		err := c.transport.Decode(request.Command, voteRequest)
		if err != nil {
			c.logger.Error("failed to decode vote request", "error", err.Error())
			response.Error = model.ErrorBadCommand
			return model.ErrorBadCommand
		}
		resp := &model.RequestVoteResponse{}
		err = c.RequestVote(voteRequest, resp)
		if err != nil {
			response.Error = err
			return err
		}
		response.CommandResponse = resp
		return nil
	case model.State:
		c.logger.Info("receive state request", "peer", request.Header.Node.ID)
		resp := &model.NodeWithState{}
		err := c.State(resp)
		if err != nil {
			response.Error = err
			return err
		}
		response.CommandResponse = resp
		return nil
	}
	return nil
}

// HeartBeat handles heartbeat request from peer node
func (c *Consensus) HeartBeat(args *model.HeartBeatRequest, reply *model.HeartBeatResponse) error {
	c.logger.Debug("receive heartbeat", "from", args.NodeId)

	if c.term > args.Term {
		c.logger.Info("peer term is behind self", "peer term", args.Term, "self term", c.term)
		// term in the request is behind this node
		model.HBResponse(reply, false, common.HeartbeatExpired.String())
		return nil
	}

	// update term of this node
	c.setTerm(args.Term)

	switch {
	case c.ensureState(model.NodeStateLeader):
		// leave leader state
		c.sendEvent(model.NodeStateLeader, model.EventLeaveLeader)
	case c.ensureState(model.NodeStateFollower):
		// send heartbeat to handler
		c.followerChan <- struct{}{}
	case c.ensureState(model.NodeStateCandidate):
		// receive a new leader
		c.sendEvent(model.NodeStateCandidate, model.EventNewLeader)
	case c.ensureState(model.NodeStateDown):
	}

	model.HBResponse(reply, true, common.HeartbeatOk.String())
	return nil
}

// RequestVote handle vote request from peer node
func (c *Consensus) RequestVote(args *model.RequestVoteRequest, reply *model.RequestVoteResponse) error {
	c.logger.Info("receive vote request", "from", args.NodeAddr, "term", args.Term, "current term", c.term)
	// return when no-vote is true
	if c.node.NoVote {
		model.VoteResponse(reply, c.node.Node, false, common.VoteNoVoteNode.String())
		return nil
	}

	switch {
	case c.ensureState(model.NodeStateLeader):
		if args.Term <= c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteLeaderExist.String())
			return nil
		}
		// term in the request is newer, leaves leader state
		c.sendEvent(model.NodeStateLeader, model.EventLeaveLeader)
	case c.ensureState(model.NodeStateFollower):
		if args.Term < c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteTermExpired.String())
			return nil
		}
	case c.ensureState(model.NodeStateCandidate):
		if args.Term <= c.term {
			model.VoteResponse(reply, c.node.Node, false, common.VoteHaveVoted.String())
			return nil
		}
		// term in the request is newer, vote and switch to follower state
		c.sendEvent(model.NodeStateCandidate, model.EventNewTerm)
	case c.ensureState(model.NodeStateDown):
	}

	// update term cache
	c.setTerm(args.Term)
	c.vote(args.Term, args.NodeId)

	c.logger.Info("vote for", "node", args.NodeId, "term", args.Term)
	model.VoteResponse(reply, c.node.Node, true, common.VoteOk.String())
	return nil
}

// State return current node state
func (c *Consensus) State(reply *model.NodeWithState) error {
	*reply = model.NodeWithState{
		State: model.NodeState(c.fsm.Current()),
		Node:  c.node,
	}
	return nil
}

// IsLeader determines whether the current node is the leader node.
func (c *Consensus) IsLeader() bool {
	// uses the state machine to check if the current node's state is that of a leader
	return c.fsm.Is(model.NodeStateLeader.String())
}

// Leader retrieves the current leader node from the cluster.
// It returns an error if no leader is found or there's an issue fetching the cluster state.
func (c *Consensus) Leader() (*model.ElectNode, error) {
	clusterState, err := c.ClusterState()
	if err != nil {
		c.logger.Error("failed to get cluster state", "error", err.Error())
		return nil, err
	}

	for _, nodeWithState := range clusterState.Nodes {
		if nodeWithState.State == model.NodeStateLeader {
			return &nodeWithState.Node, nil
		}
	}

	return nil, fmt.Errorf("no leader found")
}

// ClusterState retrieves the current state of the cluster including all nodes.
func (c *Consensus) ClusterState() (*model.ClusterState, error) {
	clusterState := &model.ClusterState{
		Nodes: map[string]*model.NodeWithState{
			c.node.ID: {
				State: model.NodeState(c.fsm.Current()),
				Node:  c.node,
			},
		},
	}
	stateMap := sync.Map{}
	g := errgroup.Group{}
	for _, peer := range c.cfg.Peers {
		if c.isSelf(peer.ID, peer.Address) {
			// skip self
			continue
		}

		peerID := peer.ID
		g.Go(func() error {
			c.logger.Info("send state request to peer", "peer", peerID)
			resp := &model.Response{}
			// send state request
			err := c.transport.SendRequest(peerID, &model.Request{
				Header:      c.buildHeaders(),
				CommandCode: model.State,
				Command:     nil,
			}, resp)
			if err != nil {
				c.logger.Error("failed to get node state", "peer", peerID, "err", err.Error())
				return fmt.Errorf("failed to get node state, peer %s, err: %s", peerID, err.Error())
			}
			stateResponse := &model.NodeWithState{}
			err = c.transport.Decode(resp.CommandResponse, stateResponse)
			if err != nil {
				c.logger.Error("failed to decode state response", "peer", peerID, "err", err.Error())
				return fmt.Errorf("failed to decode state response, peer %s, err: %s", peerID, err.Error())
			}

			stateMap.Store(resp.Node.ID, stateResponse)
			return nil
		})
	}
	err := g.Wait()
	if err != nil {
		c.logger.Error("get cluster state error", "error", err.Error())
	}

	stateMap.Range(func(key, value any) bool {
		clusterState.Nodes[key.(string)] = value.(*model.NodeWithState)
		return true
	})

	return clusterState, err
}

// CurrentState returns the current election node state.
func (c *Consensus) CurrentState() model.NodeState {
	return model.NodeState(c.fsm.Current())
}

// Visualize returns a visualization of the current consensus state machine in Graphviz format.
func (c *Consensus) Visualize() string {
	return fsm.Visualize(c.fsm)
}

func (c *Consensus) initializeTransport() error {
	// start the transport server
	err := c.transport.Start(c.node.Address, c.HandleRequest, c.transportConfig)
	if err != nil {
		c.logger.Error("failed to start transport server", "error", err.Error())
		return err
	}

	var peers []*model.Node
	for _, peer := range c.cfg.Peers {
		if c.isSelf(peer.ID, peer.Address) {
			// skip self
			continue
		}
		peers = append(peers, &model.Node{
			ID:      peer.ID,
			Address: peer.Address,
			Tags:    peer.Tags,
		})
	}

	// init transport clients
	err = c.transport.InitConnections(peers, c.transportConfig)
	if err != nil {
		c.logger.Error("failed to init clients", "error", err.Error())
		return err
	}

	c.logger.Info("success to init transport")
	return nil
}

func (c *Consensus) buildHeaders() model.Header {
	return model.Header{Node: c.node.Node}
}

func (c *Consensus) ensureState(state model.NodeState) bool {
	if model.NodeState(c.fsm.Current()) != state {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			c.logger.Error("wait for state ready timeout", "state", state)
			return false
		default:
		}
		stateReady := false
		switch state {
		case model.NodeStateLeader:
			stateReady = c.inLeaderState
		case model.NodeStateFollower:
			stateReady = c.inFollowerState
		case model.NodeStateCandidate:
			stateReady = c.inCandidateState
		case model.NodeStateDown:
			stateReady = c.inDownState
		}

		if stateReady {
			break
		}
		time.Sleep(500 * time.Microsecond)
	}

	return true
}

func (c *Consensus) enterLeader(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become leader")
	c.leaderChan = make(chan struct{}, 1)
	go func() {
		err := c.runLeader(ctx)
		if err != nil {
			c.logger.Error("failed to enter leader state", "err", err.Error())
			return
		}
	}()
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
	c.inLeaderState = true
}

func (c *Consensus) runLeader(_ context.Context) error {
	tk := time.NewTicker(c.cfg.HeartBeatInterval)
	defer tk.Stop()

	for {
		select {
		case <-c.leaderChan:
			c.logger.Info("leave leader")
			return nil
		default:
		}

		var errCount int
		// send heartbeat to followers
		c.sendHeartBeat(&errCount)
		// leaves leader state if the number of errors is more than half
		if errCount >= c.countVoteNode()/2+1 {
			c.sendEvent(model.NodeStateLeader, model.EventLeaveLeader)
		}

		select {
		case <-c.leaderChan:
			c.logger.Info("leave leader")
			return nil
		case <-tk.C:
		}
	}
}

func (c *Consensus) leaveLeader(_ context.Context, ev *fsm.Event) {
	c.logger.Info("leave leader")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.leaderChan)
	c.inLeaderState = false
}

func (c *Consensus) enterFollower(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become follower")
	c.followerChan = make(chan struct{}, 1)
	go func() {
		err := c.runFollower(ctx)
		if err != nil {
			c.logger.Error("failed to enter follower state", "err", err.Error())
			return
		}
	}()
	c.inFollowerState = true
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
}

func (c *Consensus) runFollower(_ context.Context) error {
	// set the heartbeat timeout to twice the heartbeat interval
	heartbeatTimeout := c.cfg.HeartBeatInterval * 2
	ts := time.NewTimer(heartbeatTimeout)
	defer ts.Stop()
	for {
		select {
		case _, ok := <-c.followerChan:
			if !ok {
				// channel is closed
				c.logger.Info("leave follower state, channel is closed")
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
			c.logger.Info("leave follower state due to heartbeat timeout")
			// heartbeat timeout
			c.sendEvent(model.NodeStateFollower, model.EventHeartbeatTimeout)
			return nil
		}
	}
}

func (c *Consensus) leaveFollower(_ context.Context, ev *fsm.Event) {
	c.logger.Info("leave follower")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.followerChan)
	c.inFollowerState = false
}

func (c *Consensus) enterCandidate(ctx context.Context, ev *fsm.Event) {
	c.logger.Info("become candidate")
	c.candidateChan = make(chan struct{}, 1)
	go func() {
		err := c.runCandidate(ctx)
		if err != nil {
			c.logger.Error("failed to enter candidate state", "err", err.Error())
			return
		}
	}()
	c.inCandidateState = true
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
}

func (c *Consensus) runCandidate(ctx context.Context) error {
	if c.node.NoVote {
		c.logger.Info("novote node, not participate in the election")
		return nil
	}

	err := c.tryToBecomeLeader(ctx)
	if err != nil {
		c.logger.Error("failed to make the try to become leader", "err", err.Error())
		return err
	}

	return nil
}

func (c *Consensus) tryToBecomeLeader(_ context.Context) error {
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
		voteChan := make(chan model.Node, len(c.cfg.Peers))
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
				c.sendEvent(model.NodeStateCandidate, model.EventMajorityVotes)
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
					c.sendEvent(model.NodeStateCandidate, model.EventMajorityVotes)
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

func (c *Consensus) leaveCandidate(_ context.Context, ev *fsm.Event) {
	c.logger.Info("leave candidate")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.candidateChan)
	c.inCandidateState = false
}

func (c *Consensus) enterShutdown(_ context.Context, ev *fsm.Event) {
	c.logger.Info("become shutdown")
	c.inDownState = true
	c.sendNodeStateTransition(model.NodeState(ev.Dst), model.NodeState(ev.Src), model.TransitionTypeEnter)
}

func (c *Consensus) leaveShutdown(_ context.Context, ev *fsm.Event) {
	c.logger.Info("leave shutdown")
	c.sendNodeStateTransition(model.NodeState(ev.Src), model.NodeState(ev.Dst), model.TransitionTypeLeave)
	close(c.shutdownChan)
	c.inDownState = false
}

func (c *Consensus) sendEvent(currentState model.NodeState, ev model.NodeEvent) {
	if currentState == c.preEventState {
		c.logger.Warn("event occurring simultaneously under the same state,ignore it",
			"state", currentState, "event", ev)
		return
	}
	c.preEventState = currentState
	c.eventChan <- ev
	c.logger.Debug("node event", "event", ev.String())
}

func (c *Consensus) runEventHandler() {

	handler := func(ev model.NodeEvent) {
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
	go func() {
		for ev := range c.eventChan {
			handler(ev)
		}
	}()
}

func (c *Consensus) sendHeartBeat(errorCount *int) {
	g := errgroup.Group{}
	for _, peer := range c.cfg.Peers {
		if c.isSelf(peer.ID, peer.Address) {
			// skip self
			continue
		}

		peerID := peer.ID
		g.Go(func() error {
			c.logger.Debug("send heartbeat to peer", "peer", peerID)
			resp := &model.Response{}
			// send heartbeat request to peer
			err := c.transport.SendRequest(peerID, &model.Request{
				Header:      c.buildHeaders(),
				CommandCode: model.HeartBeat,
				Command:     &model.HeartBeatRequest{NodeId: c.node.ID, Term: c.term},
			}, resp)
			if err != nil {
				// error count
				*errorCount += 1
				c.logger.Error("failed to send heartbeat", "peer", peerID, "error", err.Error())
				return fmt.Errorf("heartbeat, failed to send heartbeat, peer %s, error %s", peerID, err.Error())
			}
			hbResponse := &model.HeartBeatResponse{}
			err = c.transport.Decode(resp.CommandResponse, hbResponse)
			if err != nil {
				*errorCount += 1
				return fmt.Errorf("heartbeat, peer %s, bad response", peerID)
			}
			if !hbResponse.Ok {
				// error count
				*errorCount += 1
				return fmt.Errorf("heartbeat, peer %s response not ok, message %s", peerID, hbResponse.Message)
			}

			c.logger.Debug("send heartbeat to peer", "peer", peerID)
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
	for _, peer := range c.cfg.Peers {
		if c.isSelf(peer.ID, peer.Address) {
			// skip self
			continue
		}

		peerID := peer.ID
		g.Go(func() error {
			if !c.ensureState(model.NodeStateCandidate) {
				return nil
			}

			c.logger.Info("send vote request to peer", "peer", peerID)
			resp := &model.Response{}
			// send vote request
			err := c.transport.SendRequest(peerID, &model.Request{
				Header:      c.buildHeaders(),
				CommandCode: model.RequestVote,
				Command: model.RequestVoteRequest{
					NodeId:   c.node.ID,
					Term:     c.term,
					NodeAddr: c.node.Address,
				},
			}, resp)
			if err != nil {
				c.logger.Error("failed to get vote", "peer", peerID, "err", err.Error())
				return fmt.Errorf("failed to get vote, peer %s, err: %s", peerID, err)
			}
			voteResp := &model.RequestVoteResponse{}
			err = c.transport.Decode(resp.CommandResponse, voteResp)
			if err != nil {
				c.logger.Error("vote response, bad response", "peer", peerID, "err", err.Error())
				return fmt.Errorf("vote response, peer %s, bad response: %s", peerID, err)
			}

			if !voteResp.Vote {
				c.logger.Info("peer disagrees with voting for you", "peer", peerID, "message", voteResp.Message)
				return nil
			}

			select {
			case <-c.candidateChan:
				return nil
			case voteChan <- resp.Node:
				// receive a valid vote from peer node
			}
			c.logger.Info("get voting", "peer", peerID)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		c.logger.Error("candidate, request voting error", "error", err.Error())
		return err
	}

	return nil
}

func (c *Consensus) isSelf(nodeID, nodeAddress string) bool {
	return c.node.ID == nodeID || c.node.Address == nodeAddress
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
