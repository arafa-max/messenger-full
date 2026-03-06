package chat

import "time"

type Message struct{
	ID string
	From string
	To string
	Text string
	SentAt time.Time
}
