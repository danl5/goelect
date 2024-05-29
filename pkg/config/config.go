package config

import (
	"time"
)

// Config represents the elect config
type Config struct {
	// ElectTimeout is timeout duration for leader election
	ElectTimeout time.Duration `json:"elect_timeout,omitempty"`
	// HeartBeatInterval is interval duration for heartbeat between leader and followers
	HeartBeatInterval time.Duration `json:"heartbeat_timeout,omitempty"`
	// ConnectTimeout represents the timeout duration for a transport connection
	ConnectTimeout time.Duration `json:"connect_timeout,omitempty"`
	// Peers contain information about all nodes in the cluster.
	Peers []NodeConfig `json:"peers,omitempty"`
}

type NodeConfig struct {
	// ID of node
	ID string `json:"id"`
	// Address of node, used for establishing connections
	Address string `json:"address"`
	// NoVote represents whether the node participates in voting or not
	NoVote bool `json:"no_vote"`
	// Tags represent additional label information of the node
	Tags map[string]string `json:"tags"`
}
