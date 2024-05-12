package consensus

import (
	"log/slog"
	"testing"

	"github.com/looplab/fsm"
	"github.com/stretchr/testify/assert"

	"github.com/danl5/goelect/pkg/common"
	"github.com/danl5/goelect/pkg/log"
	"github.com/danl5/goelect/pkg/model"
)

func TestConsensus_HeartBeat(t *testing.T) {
	type fields struct {
		termCache *termCache
		logger    log.Logger
		eventChan chan model.NodeEvent
	}
	type args struct {
		args  *model.HeartBeatRequest
		reply *model.HeartBeatResponse
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		result  *model.HeartBeatResponse
	}{
		{
			name: "normal_heartbeat",
			fields: fields{
				termCache: &termCache{
					term: 1,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
			},
			args: args{
				args: &model.HeartBeatRequest{
					Term: 2,
				},
				reply: &model.HeartBeatResponse{},
			},
			result: &model.HeartBeatResponse{
				Ok:      true,
				Message: common.HeartbeatOk.String(),
			},
			wantErr: false,
		},
		{
			name: "expired_heartbeat",
			fields: fields{
				termCache: &termCache{
					term: 2,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
			},
			args: args{
				args: &model.HeartBeatRequest{
					Term: 1,
				},
				reply: &model.HeartBeatResponse{},
			},
			result: &model.HeartBeatResponse{
				Ok:      false,
				Message: common.HeartbeatExpired.String(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &RpcHandler{
				Consensus: &Consensus{
					termCache: tt.fields.termCache,
					logger:    tt.fields.logger,
					eventChan: tt.fields.eventChan,
				},
			}
			if err := c.HeartBeat(tt.args.args, tt.args.reply); (err != nil) != tt.wantErr {
				t.Errorf("HeartBeat() error = %v, wantErr %v", err, tt.wantErr)
			}

			assert.Equal(t, tt.args.reply.Ok, tt.result.Ok)
			assert.Equal(t, tt.args.reply.Message, tt.result.Message)
		})
	}
}

func TestConsensus_RequestVote(t *testing.T) {
	type fields struct {
		termCache *termCache
		logger    log.Logger
		fsm       *fsm.FSM
		eventChan chan model.NodeEvent
	}
	type args struct {
		args  *model.RequestVoteRequest
		reply *model.RequestVoteResponse
	}
	type result struct {
		vote    bool
		message string
	}
	leaderFsm := &fsm.FSM{}
	leaderFsm.SetState(model.NodeStateLeader.String())

	followerFsm := &fsm.FSM{}
	followerFsm.SetState(model.NodeStateFollower.String())

	candidateFsm := &fsm.FSM{}
	candidateFsm.SetState(model.NodeStateCandidate.String())

	tests := []struct {
		name    string
		fields  fields
		args    args
		res     result
		wantErr bool
	}{
		{
			name: "vote_leader_ok",
			fields: fields{
				termCache: &termCache{
					term: 1,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
				fsm:       leaderFsm,
			},
			args: args{
				args: &model.RequestVoteRequest{
					Term: 2,
				},
				reply: &model.RequestVoteResponse{},
			},
			res: result{
				vote:    true,
				message: common.VoteOk.String(),
			},
			wantErr: false,
		},
		{
			name: "vote_leader_exist",
			fields: fields{
				termCache: &termCache{
					term: 1,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
				fsm:       leaderFsm,
			},
			args: args{
				args: &model.RequestVoteRequest{
					Term: 1,
				},
				reply: &model.RequestVoteResponse{},
			},
			res: result{
				vote:    false,
				message: common.VoteLeaderExist.String(),
			},
			wantErr: false,
		},
		{
			name: "vote_follower_ok",
			fields: fields{
				termCache: &termCache{
					term: 1,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
				fsm:       followerFsm,
			},
			args: args{
				args: &model.RequestVoteRequest{
					Term: 2,
				},
				reply: &model.RequestVoteResponse{},
			},
			res: result{
				vote:    true,
				message: common.VoteOk.String(),
			},
			wantErr: false,
		},
		{
			name: "vote_follower_expired",
			fields: fields{
				termCache: &termCache{
					term: 2,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
				fsm:       followerFsm,
			},
			args: args{
				args: &model.RequestVoteRequest{
					Term: 1,
				},
				reply: &model.RequestVoteResponse{},
			},
			res: result{
				vote:    false,
				message: common.VoteTermExpired.String(),
			},
			wantErr: false,
		},
		{
			name: "vote_candidate_ok",
			fields: fields{
				termCache: &termCache{
					term: 1,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
				fsm:       candidateFsm,
			},
			args: args{
				args: &model.RequestVoteRequest{
					Term: 2,
				},
				reply: &model.RequestVoteResponse{},
			},
			res: result{
				vote:    true,
				message: common.VoteOk.String(),
			},
			wantErr: false,
		},
		{
			name: "vote_candidate_voted",
			fields: fields{
				termCache: &termCache{
					term: 2,
				},
				logger:    slog.Default(),
				eventChan: make(chan model.NodeEvent, 10),
				fsm:       candidateFsm,
			},
			args: args{
				args: &model.RequestVoteRequest{
					Term: 2,
				},
				reply: &model.RequestVoteResponse{},
			},
			res: result{
				vote:    false,
				message: common.VoteHaveVoted.String(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &RpcHandler{
				Consensus: &Consensus{
					termCache: tt.fields.termCache,
					logger:    tt.fields.logger,
					fsm:       tt.fields.fsm,
					eventChan: tt.fields.eventChan,
				},
			}
			if err := c.RequestVote(tt.args.args, tt.args.reply); (err != nil) != tt.wantErr {
				t.Errorf("RequestVote() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.args.reply.Vote, tt.res.vote)
			assert.Equal(t, tt.args.reply.Message, tt.res.message)
		})
	}
}
