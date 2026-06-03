package handlers

import "net/http"

type Renderer interface {
	Render(w http.ResponseWriter, name string, data interface{})
}
