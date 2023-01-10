package cadet

import (
	"encoding/json"
	"net/http"
)

type Request struct {
	command     *Command
	RawResponse http.ResponseWriter
	RawRequest  *http.Request
}

func (c *Request) ReadCommand(obj any) error {
	return json.Unmarshal(c.command.Data, obj)
}
