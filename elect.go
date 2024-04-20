package goelect

import (
	"context"
	"time"

	"github.com/danli001/goelect/internal/config"
	"github.com/danli001/goelect/internal/consensus"
	"github.com/danli001/goelect/internal/log"
	"github.com/danli001/goelect/internal/model"
	"github.com/danli001/goelect/internal/rpc"
)

const (
	// election timeout
	defaultElectTimeout = 200

	// heartbeat interval
	defaultHeartBeatInterval = 150

	// connect timeout
	defaultConnectTimeout = 5
)

// NewElect creates a new Elect instance
func NewElect(cfg *ElectConfig, logger log.Logger) (*Elect, error) {
	var peers []config.NodeConfig
	for _, n := range cfg.Peers {
		peers = append(peers, config.NodeConfig{
			ID:      n.ID,
			Address: n.Address,
			Tags:    n.Tags,
			NoVote:  n.NoVote,
		})
	}

	electTimeout := cfg.ElectTimeout
	if cfg.ElectTimeout == 0 {
		electTimeout = defaultElectTimeout
	}
	heartbeatInterval := cfg.HeartBeatInterval
	if cfg.HeartBeatInterval == 0 {
		heartbeatInterval = defaultHeartBeatInterval
	}
	connectTimeout := cfg.ConnectTimeout
	if cfg.ConnectTimeout == 0 {
		connectTimeout = defaultConnectTimeout
	}

	// new consensus instance
	c, err := consensus.NewConsensus(&config.Config{
		ElectTimeout:      time.Duration(electTimeout) * time.Millisecond,
		HeartBeatInterval: time.Duration(heartbeatInterval) * time.Millisecond,
		ConnectTimeout:    time.Duration(connectTimeout) * time.Second,
		Peers:             peers,
	}, logger, model.ElectNode{
		Node: model.Node{
			Address: cfg.Node.Address,
			ID:      cfg.Node.ID,
			Tags:    cfg.Node.Tags,
		},
		NoVote: cfg.Node.NoVote,
	})
	if err != nil {
		return nil, err
	}
	return &Elect{
		cfg:             cfg,
		logger:          logger,
		callBackTimeout: cfg.CallBackTimeout,
		consensus:       c,
		callBacks:       cfg.CallBacks,
		errChan:         make(chan error, 10),
	}, nil
}

// Elect contains information about an election
type Elect struct {
	// callBacks stores the callbacks to be triggered when the state changes
	callBacks *StateCallBacks
	// callBackTimeout is the timeout for the callbacks
	callBackTimeout int
	// consensus is the consensus algorithm used for the election
	consensus *consensus.Consensus
	// errChan is a channel for errors
	errChan chan error

	// cfg is the configuration for the election
	cfg *ElectConfig
	// logger is used for logging
	logger log.Logger
}

// Run is the main function of the Elect struct
// It starts the RPC server, runs the consensus algorithm.
func (e *Elect) Run() error {
	// start the RPC server
	err := e.startServer()
	if err != nil {
		e.logger.Error("elect, failed to start rpc server", "error", err.Error())
		return err
	}

	// run the consensus algorithm
	stateChan, err := e.consensus.Run()
	if err != nil {
		e.logger.Error("elect, failed to run elect", "error", err.Error())
		return err
	}
	// handle state transitions in a separate goroutine
	go e.handleStateTransition(stateChan)

	e.logger.Info("elect, elect started")
	return nil
}

// Errors returns a receive-only channel of type error from the Elect struct
func (e *Elect) Errors() <-chan error {
	// return the error channel from the Elect struct
	return e.errChan
}

// CurrentState return current node state
func (e *Elect) CurrentState() string {
	return e.consensus.CurrentState().String()
}

func (e *Elect) startServer() error {
	rpcSvr, err := rpc.NewRpcServer(e.logger)
	if err != nil {
		return err
	}

	go func() {
		err = rpcSvr.Start(e.cfg.Node.Address, e.consensus)
		if err != nil {
			e.logger.Error("elect, failed to start rpc server", "error", err.Error())
			return
		}
	}()

	e.logger.Info("start rpc server")
	return nil
}

func (e *Elect) sendError(err error) {
	select {
	case e.errChan <- err:
	default:
	}
}

func (e *Elect) handleStateTransition(stateChan <-chan model.StateTransition) {
	for {
		select {
		case st, ok := <-stateChan:
			if !ok {
				e.logger.Info("elect, state transition chan is closed")
				return
			}

			e.logger.Debug("elect, elect state transition", "type", st.Type.String(), "state", st.State, "src", st.SrcState)
			var err error
			switch st.Type {
			case model.TransitionTypeLeave:
				switch st.State {
				case model.NodeStateLeader:
					err = e.execStateHandler(e.callBacks.LeaveLeader, st)
				case model.NodeStateFollower:
					err = e.execStateHandler(e.callBacks.LeaveFollower, st)
				case model.NodeStateCandidate:
					err = e.execStateHandler(e.callBacks.LeaveCandidate, st)
				}
			case model.TransitionTypeEnter:
				switch st.State {
				case model.NodeStateLeader:
					err = e.execStateHandler(e.callBacks.EnterLeader, st)
				case model.NodeStateFollower:
					err = e.execStateHandler(e.callBacks.EnterFollower, st)
				case model.NodeStateCandidate:
					err = e.execStateHandler(e.callBacks.EnterCandidate, st)
				}
			default:
			}
			if err != nil {
				e.sendError(err)
			}
		}
	}
}

func (e *Elect) execStateHandler(sh StateHandler, st model.StateTransition) error {
	if sh == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.callBackTimeout)*time.Second)
	defer cancel()

	err := sh(ctx, st)
	if err != nil {
		return err
	}

	e.logger.Debug("callback end")
	return nil
}

// ElectConfig is a struct that represents the configuration for an election.
type ElectConfig struct {
	// Timeout for heartbeat messages, in milliseconds
	HeartBeatInterval uint
	// Timeout for election messages, in milliseconds
	ElectTimeout uint
	// Timeout for connecting to peers, in seconds
	ConnectTimeout uint
	// List of peers in the network
	Peers []Node
	// Node information
	Node Node
	// State callbacks
	CallBacks *StateCallBacks
	// Timeout for callbacks
	CallBackTimeout int
}

// Node is a struct that represents an elect node
type Node struct {
	// ID of the node
	ID string
	// Address of the node
	Address string
	// NoVote indicates whether the node is able to vote
	NoVote bool
	// Tags associated with the node
	Tags map[string]string
}

type StateHandler func(ctx context.Context, st model.StateTransition) error

// StateCallBacks is a struct to hold state callbacks
type StateCallBacks struct {
	// EnterLeader is a callback function to be called when entering the leader state
	EnterLeader StateHandler
	// LeaveLeader is a callback function to be called when leaving the leader state
	LeaveLeader StateHandler
	// EnterFollower is a callback function to be called when entering the follower state
	EnterFollower StateHandler
	// LeaveFollower is a callback function to be called when leaving the follower state
	LeaveFollower StateHandler
	// EnterCandidate is a callback function to be called when entering the candidate state
	EnterCandidate StateHandler
	// LeaveCandidate is a callback function to be called when leaving the candidate state
	LeaveCandidate StateHandler
}
