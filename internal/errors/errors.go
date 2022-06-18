package errors

import (
	"encoding/json"
)

type BusinessErr struct {
	target  string
	message string
}

func (e *BusinessErr) Error() string {
	return e.message
}

func (e *BusinessErr) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Target  string `json:"target"`
		Message string `json:"message"`
	}{Target: e.target, Message: e.message})
}

func NewBusinessErr(target string, msg string) error {
	return &BusinessErr{
		target:  target,
		message: msg,
	}
}

type EntryNotFoundErr struct {
	message string
}

func (e *EntryNotFoundErr) Error() string {
	return e.message
}

func NewEntryNotFoundErr(msg string) *EntryNotFoundErr {
	return &EntryNotFoundErr{message: msg}
}
