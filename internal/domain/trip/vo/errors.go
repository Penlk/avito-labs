package vo

type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Reason }
