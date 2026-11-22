package push

import "time"

type Response struct {
	MessageId  string    `json:"message_id"`
	AcceptedAt time.Time `json:"accepted_at"`
}
