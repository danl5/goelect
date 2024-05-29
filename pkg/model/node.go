package model

import (
	"errors"
)

// NodeState represents the state of a node in a distributed system.
type NodeState string

const (
	// NodeStateLeader leader state
	NodeStateLeader NodeState = "leader"
	// NodeStateFollower follower state
	NodeStateFollower NodeState = "follower"
	// NodeStateCandidate candidate state
	NodeStateCandidate NodeState = "candidate"
	// NodeStateDown down state
	NodeStateDown NodeState = "down"
)

func (n NodeState) String() string {
	return string(n)
}

// Node represents a node instance
type Node struct {
	ID      string            `json:"id" mapstructure:"id"`
	Address string            `json:"address" mapstructure:"address"`
	Tags    map[string]string `json:"tags" mapstructure:"tags"`
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

// ElectNode represents a node instance with elect meta
type ElectNode struct {
	Node `json:"node" mapstructure:"node"`

	NoVote bool `json:"no_vote" mapstructure:"no_vote"`
}
