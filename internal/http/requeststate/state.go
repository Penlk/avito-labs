package requeststate

import "context"

type key struct{}

type State struct {
	Route        string
	Status       int
	Err          error
	ErrorType    string
	TraceContext context.Context
}

func With(ctx context.Context, state *State) context.Context {
	return context.WithValue(ctx, key{}, state)
}

func From(ctx context.Context) *State {
	state, _ := ctx.Value(key{}).(*State)
	return state
}

func Record(ctx context.Context, err error, kind string) {
	if state := From(ctx); state != nil {
		state.Err, state.ErrorType = err, kind
	}
}
