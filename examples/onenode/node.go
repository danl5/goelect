package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/danl5/goelect"
	"github.com/danl5/goelect/pkg/model"
)

var (
	// nodeAddress stores the address of the self node
	nodeAddress = flag.String("nodeaddr", "127.0.0.1:9981", "self node address")

	// peers stores the addresses of the peers nodes separated by a comma
	peers = flag.String("peers", "127.0.0.1:9981", "peers node address separated by comma")
)

// Callback functions for state transitions
func enterLeader(ctx context.Context, st model.StateTransition) error {
	fmt.Println("enter leader,", st.State, st.SrcState)
	return nil
}

func leaveLeader(ctx context.Context, st model.StateTransition) error {
	fmt.Println("leave leader,", st.State, st.SrcState)
	return nil
}

func enterFollower(ctx context.Context, st model.StateTransition) error {
	fmt.Println("enter follower,", st.State, st.SrcState)
	return nil
}

func leaveFollower(ctx context.Context, st model.StateTransition) error {
	fmt.Println("leave follower,", st.State, st.SrcState)
	return nil
}

func enterCandidate(ctx context.Context, st model.StateTransition) error {
	fmt.Println("enter candidate,", st.State, st.SrcState)
	return nil
}

func leaveCandidate(ctx context.Context, st model.StateTransition) error {
	fmt.Println("leave candidate,", st.State, st.SrcState)
	return nil
}

func newElect() (*goelect.Elect, error) {
	pAddrs := strings.Split(*peers, ",")
	if len(pAddrs) == 0 {
		panic("peers is empty")
	}

	var peerNodes []goelect.Node
	for _, pa := range pAddrs {
		peerNodes = append(peerNodes, goelect.Node{Address: pa, ID: pa})
	}

	e, err := goelect.NewElect(&goelect.ElectConfig{
		ElectTimeout:      200,
		HeartBeatInterval: 150,
		ConnectTimeout:    10,
		Peers:             peerNodes,
		// state transition callbacks
		CallBacks: &goelect.StateCallBacks{
			EnterLeader:    enterLeader,
			LeaveLeader:    leaveLeader,
			EnterFollower:  enterFollower,
			LeaveFollower:  leaveFollower,
			EnterCandidate: enterCandidate,
			LeaveCandidate: leaveCandidate,
		},
		// self node
		Node: goelect.Node{
			Address: *nodeAddress,
			ID:      *nodeAddress,
		},
	}, slog.Default())
	if err != nil {
		return nil, err
	}

	return e, nil
}

func main() {
	flag.Parse()

	e, err := newElect()
	if err != nil {
		panic(err)
	}

	// run the elect
	go func() {
		err = e.Run()
		if err != nil {
			panic(err)
		}
	}()

	tk := time.NewTicker(5 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-tk.C:
			cs, _ := e.ClusterState()
			fmt.Println("Node\tState\t")
			for addr, n := range cs.Nodes {
				fmt.Println(addr, n.State.String())
			}
			fmt.Println()
			leaderNode, _ := e.Leader()
			fmt.Println("Leader:", leaderNode)

			fmt.Println()
			isLeader := e.IsLeader()
			fmt.Println("IsLeader:", isLeader)
			fmt.Println()
		}
	}
}
