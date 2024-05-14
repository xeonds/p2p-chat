package model

import "time"

type Message struct {
	Id  int       `json:"id"`
	Ts  time.Time `json:"ts"`
	Msg string    `json:"msg"`
}
