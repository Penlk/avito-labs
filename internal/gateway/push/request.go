package push

import "github.com/google/uuid"

const (
	requestPositionKind = "REQUEST_POSITION"
)

type Data struct {
	TripId uuid.UUID `json:"trip_id"`
}

type Request struct {
	RequestId   string `json:"request_id"`
	RecipientId string `json:"recipient_id"`
	Data        Data   `json:"data"`

	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`

	Kind string `json:"kind"`
}

func NewRequest(requestId, recipientId string, data Data) Request {
	return Request{
		RequestId:   requestId,
		RecipientId: recipientId,
		Data:        data,

		Kind: requestPositionKind,
	}
}
