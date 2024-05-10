<h1 align="center" style="border-bottom: none">
    <a><img alt="goelect" src="/docs/goelect-logo.svg"></a>
</h1>

[![Go Report Card](https://goreportcard.com/badge/github.com/danl5/goelect)](https://goreportcard.com/report/github.com/danl5/goelect) ![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/danl5/goelect?sort=semver)

Goelect is an open-source Go (Golang) library for leader election. It is heavily influenced by the election component of the Raft implementation. For more details, you can refer to [Raft Wiki](https://en.wikipedia.org/wiki/Raft_(algorithm)).

## Features
* **Independent Operation**: No third-party services are required. You don't need to set up or rely on external systems like ZooKeeper or etcd.
* **Simplified Integration**: Easy to integrate into your existing Golang projects with minimal configuration.
* **Supports Novote role**：The no-vote node does not participate in the election.
* **Highly Available**: Built to be fault-tolerant, suitable for systems that require high availability.

## How to use
### Config
```go
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
    // Node represents the information of this node
    Node Node
    // State callbacks
    CallBacks *StateCallBacks
    // Timeout for callbacks, in seconds
    CallBackTimeout int
}
```
### Example
`examples/onenode/node.go` is a great example of using this goelect package. 

Create an Elect instance:
```go
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
```
Start the Elect:
```go
err = e.Run()
```
This is everything we need to do :)
