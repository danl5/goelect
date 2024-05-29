package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/danl5/goelect/pkg/config"
	"github.com/danl5/goelect/pkg/consensus"
	"github.com/danl5/goelect/pkg/model"
	"github.com/danl5/goelect/pkg/transport/rpc"
)

var (
	outputPath = flag.String("o", "./fsm_visual", "output path")
)

func main() {
	rpcTransport, _ := rpc.NewRPC(slog.Default())
	c, _ := consensus.NewConsensus(
		model.ElectNode{
			Node: model.Node{
				ID:      "test",
				Address: "test",
			},
		},
		rpcTransport,
		&rpc.Config{},
		&config.Config{
			ConnectTimeout: 10 * time.Second,
			Peers:          []config.NodeConfig{},
		},
		nil)
	visualStr := c.Visualize()

	f, err := os.OpenFile(*outputPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	_, err = f.WriteString(visualStr)
	if err != nil {
		panic(err)
	}

	fmt.Println("Visualization finished")
}
