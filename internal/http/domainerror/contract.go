package domainerror

type errorRecorder interface {
	RecordError(error)
}
