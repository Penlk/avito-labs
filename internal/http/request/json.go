package request

import (
	"encoding/json"
	"errors"
	"io"
)

func DecodeJSON[T any](reader io.Reader, body *T) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(body); err != nil {
		return err
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON object")
	}

	return nil
}
