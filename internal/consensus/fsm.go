package consensus

import (
	"github.com/looplab/fsm"

	"github.com/danli001/goelect/internal/model"
)

var (
	nodeFsm = fsm.NewFSM(
		model.NodeStateFollower.String(),
		fsm.Events{
			{
				Name: model.EventHeartbeatTimeout.String(),
				Src:  []string{model.NodeStateFollower.String()},
				Dst:  model.NodeStateCandidate.String(),
			},
			{
				Name: model.EventLeaveLeader.String(),
				Src:  []string{model.NodeStateLeader.String()},
				Dst:  model.NodeStateFollower.String(),
			},
			{
				Name: model.EventMajorityVotes.String(),
				Src:  []string{model.NodeStateCandidate.String()},
				Dst:  model.NodeStateLeader.String(),
			},
			{
				Name: model.EventNewLeader.String(),
				Src:  []string{model.NodeStateCandidate.String()},
				Dst:  model.NodeStateFollower.String(),
			},
			{
				Name: model.EventDown.String(),
				Src: []string{
					model.NodeStateLeader.String(),
					model.NodeStateFollower.String(),
					model.NodeStateCandidate.String(),
				},
				Dst: model.NodeStateDown.String(),
			},
		},
		fsm.Callbacks{},
	)
)
