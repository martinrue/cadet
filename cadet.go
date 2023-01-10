package cadet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type ContentType int

const (
	ContentTypeUnknown ContentType = iota
	ContentTypeMultipart
	ContentTypeJSON
)

type Config struct {
	Bind string
	Path string
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

type Command struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type Server[T any] struct {
	httpServer *http.Server
	handlers   map[string]func(*Request, T) Response
	path       string
	context    T
	strictMode bool
}

func NewServer[T any](config *Config, context T) *Server[T] {
	if !strings.HasPrefix(config.Path, "/") {
		config.Path = "/" + config.Path
	}

	mux := http.NewServeMux()

	server := &Server[T]{
		httpServer: &http.Server{
			Addr:         config.Bind,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		handlers: make(map[string]func(*Request, T) Response),
		path:     config.Path,
		context:  context,
	}

	mux.HandleFunc(config.Path, server.withStrictPath()(server.executeHandler))

	return server
}

func (s *Server[T]) Use(middleware ...Middleware) {
	handler := s.executeHandler
	middleware = append([]Middleware{s.withStrictPath()}, middleware...)

	for i, j := 0, len(middleware)-1; i < j; i, j = i+1, j-1 {
		middleware[i], middleware[j] = middleware[j], middleware[i]
	}

	for _, mw := range middleware {
		handler = mw(handler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(s.path, handler)
	s.httpServer.Handler = mux
}

func (s *Server[T]) Command(name string, handler func(r *Request, context T) Response) {
	s.handlers[name] = handler
}

func (s *Server[T]) Commands(args ...any) error {
	if len(args) == 0 || len(args)%2 != 0 {
		return errors.New("args must be pairs of command names and handlers")
	}

	currentName := ""

	for i, arg := range args {
		isName := i%2 == 0

		if isName {
			name, ok := arg.(string)
			if !ok {
				return errors.New("even arg must be command name")
			}

			currentName = name
		}

		if !isName {
			handler, ok := arg.(func(*Request, T) Response)
			if !ok {
				return errors.New("odd arg must be command handler")
			}

			s.Command(currentName, handler)
			currentName = ""
		}
	}

	return nil
}

func (s *Server[T]) Handler() http.Handler {
	return s.httpServer.Handler
}

func (s *Server[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.strictMode = true
	s.httpServer.Handler.ServeHTTP(w, r)
}

func (s *Server[T]) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server[T]) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server[T]) getContentType(r *http.Request) ContentType {
	contentTypes := map[string]ContentType{
		"application/json":    ContentTypeJSON,
		"multipart/form-data": ContentTypeMultipart,
	}

	contentType, valid := contentTypes[strings.ToLower(strings.Split(r.Header.Get("Content-Type"), ";")[0])]
	if !valid {
		return ContentTypeUnknown
	}

	return contentType
}

func (s *Server[T]) getHandler(r *http.Request, contentType ContentType) (func(*Request, T) Response, *Command, error) {
	var data []byte

	if contentType == ContentTypeJSON {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, nil, err
		}

		data = body
	}

	if contentType == ContentTypeMultipart {
		body := r.FormValue("command")
		if body == "" {
			return nil, nil, errors.New("no JSON payload found in multipart request")
		}

		data = []byte(body)
	}

	command := &Command{}
	if err := json.Unmarshal(data, command); err != nil {
		return nil, nil, err
	}

	handler := s.handlers[command.Name]
	if handler == nil {
		return nil, nil, nil
	}

	return handler, command, nil
}

func (s *Server[T]) withStrictPath() Middleware {
	return func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !s.strictMode && s.path == "/" && r.URL.Path != "/" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			h.ServeHTTP(w, r)
		}
	}
}

func (s *Server[T]) executeHandler(w http.ResponseWriter, r *http.Request) {
	contentType := s.getContentType(r)
	if contentType == ContentTypeUnknown {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	if r.Method != "POST" {
		w.Header().Add("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	handler, command, err := s.getHandler(r, contentType)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	if handler == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	responder := handler(&Request{command, w, r}, s.context)
	if responder != nil {
		responder(w)
	}
}
