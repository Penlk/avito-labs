package trip

type ActiveState struct{}

func (a ActiveState) String() string {
	return "active"
}

type CompletedState struct{}

func (c CompletedState) String() string {
	return "completed"
}

func (a ActiveState) TryComplete(core *TripStatusCore) bool {
	core.updateState(CompletedState{})
	return true
}

func (c CompletedState) TryComplete(core *TripStatusCore) bool {
	return false
}
