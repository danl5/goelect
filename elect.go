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

	c, err := consensus.NewConsensus(&config.Config{
		HeartBeatInterval: cfg.HeartBeatInterval,
		ConnectTimeout:    cfg.ConnectTimeout,
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
		errChan:         make(chan error, 6),
	}, nil
}

type Elect struct {
	cfg    *ElectConfig
	logger log.Logger

	callBacks       *StateCallBacks
	callBackTimeout int
	consensus       *consensus.Consensus
	errChan         chan error
}

func (e *Elect) Run() error {
	err := e.startServer()
	if err != nil {
		e.logger.Error("elect, failed to start rpc server", "error", err.Error())
		return err
	}

	stateChan, err := e.consensus.Run()
	if err != nil {
		e.logger.Error("elect, failed to run elect", "error", err.Error())
		return err
	}
	go e.handleStateTransition(stateChan)

	e.logger.Info("elect, elect started")
	return nil
}

func (e *Elect) Errors() <-chan error {
	return e.errChan
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
			//
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

type ElectConfig struct {
	HeartBeatInterval uint
	ConnectTimeout    uint
	Peers             []Node
	Node              Node
	CallBacks         *StateCallBacks
	CallBackTimeout   int
}

type Node struct {
	ID      string
	Address string
	NoVote  bool
	Tags    map[string]string
}

type StateHandler func(ctx context.Context, st model.StateTransition) error

type StateCallBacks struct {
	EnterLeader    StateHandler
	LeaveLeader    StateHandler
	EnterFollower  StateHandler
	LeaveFollower  StateHandler
	EnterCandidate StateHandler
	LeaveCandidate StateHandler
}
