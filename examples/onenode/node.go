package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"strings"

	"github.com/danli001/goelect"
	"github.com/danli001/goelect/internal/model"
)

var (
	nodeAddress = flag.String("nodeaddr", "127.0.0.1:9981", "self node address")
	peers       = flag.String("peers", "127.0.0.1:9981", "peers node address separated by comma")
)

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

func main() {
	flag.Parse()

	pAddrs := strings.Split(*peers, ",")
	if len(pAddrs) == 0 {
		panic("peers is empty")
	}

	var peerNodes []goelect.Node
	for _, pa := range pAddrs {
		peerNodes = append(peerNodes, goelect.Node{Address: pa, ID: pa})
	}

	e, err := goelect.NewElect(&goelect.ElectConfig{
		HeartBeatInterval: 2,
		ConnectTimeout:    10,
		Peers:             peerNodes,
		CallBacks: &goelect.StateCallBacks{
			EnterLeader:    enterLeader,
			LeaveLeader:    leaveLeader,
			EnterFollower:  enterFollower,
			LeaveFollower:  leaveFollower,
			EnterCandidate: enterCandidate,
			LeaveCandidate: leaveCandidate,
		},
		Node: goelect.Node{
			Address: *nodeAddress,
			ID:      *nodeAddress,
		},
	}, slog.Default())
	if err != nil {
		panic(err)
	}

	err = e.Run()
	if err != nil {
		panic(err)
	}

	<-make(chan struct{})
}
