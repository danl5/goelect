package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/danl5/goelect/pkg/config"
	"github.com/danl5/goelect/pkg/consensus"
	"github.com/danl5/goelect/pkg/model"
)

var (
	outputPath = flag.String("o", "./fsm_visual", "output path")
)

func main() {
	c, _ := consensus.NewConsensus(&config.Config{
		ConnectTimeout: 10 * time.Second,
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
