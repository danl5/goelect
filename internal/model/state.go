package model

type TransitionType int

const (
	TransitionTypeEnter TransitionType = iota
	TransitionTypeLeave
	TransitionTypeAfter
)

func (t TransitionType) String() string {
	switch t {
	case TransitionTypeEnter:
		return "enter"
	case TransitionTypeLeave:
		return "leave"
	case TransitionTypeAfter:
		return "after"
	default:
		return "unknown"
	}
}

// StateTransition represents a transition from one state to another
type StateTransition struct {
	// State is the destination state of the transition
	State NodeState
	// SrcState is the source state of the transition
	SrcState NodeState
	// Type is the type of the transition
	Type TransitionType
}
