package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/danli001/goelect/internal/config"
	"github.com/danli001/goelect/internal/consensus"
	"github.com/danli001/goelect/internal/model"
)

var (
	outputPath = flag.String("o", "./fsm_visual", "output path")
)

func main() {
	c, _ := consensus.NewConsensus(&config.Config{
		ConnectTimeout: 10,
		Peers:          []config.NodeConfig{},
	}, nil, model.ElectNode{})
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
