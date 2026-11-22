package validation

type Error struct {
	Field  string
	Reason string
}

func (e Error) Error() string { return e.Field + ": " + e.Reason }
