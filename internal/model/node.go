package model

import (
	"errors"
)

// NodeState represents the state of a node in a distributed system.
type NodeState string

const (
	NodeStateLeader    NodeState = "leader"
	NodeStateFollower  NodeState = "follower"
	NodeStateCandidate NodeState = "candidate"
	NodeStateDown      NodeState = "down"
)

func (n NodeState) String() string {
	return string(n)
}

type Node struct {
	ID      string
	Address string
	Tags    map[string]string
}

func (n *Node) Validate() error {
	if n.ID == "" {
		return errors.New("node ID is required")
	}
	if n.Address == "" {
		return errors.New("node address is required")
	}
	return nil
}

type ElectNode struct {
	Node

	State  NodeState
	NoVote bool
}
