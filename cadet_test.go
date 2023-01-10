package cadet_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martinrue/cadet"
)

func assertEqual(t *testing.T, value any, expected any) {
	t.Helper()

	if value != expected {
		t.Fatalf("Expected %v, got %v", expected, value)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type formData map[string]string
type jsonRequestFn func(method, path, json string, contentType ...string) (*http.Response, error)
type multipartRequestFn func(method, path string, form formData) (*http.Response, error)

func createJSONRequest[T any](t *testing.T, config *cadet.Config, context T, middleware ...cadet.Middleware) (*cadet.Server[T], jsonRequestFn) {
	t.Helper()

	server := cadet.NewServer(config, context)
	server.Use(middleware...)

	httpServer := httptest.NewServer(server.Handler())

	return server, func(method, path, json string, contentType ...string) (*http.Response, error) {
		req, err := http.NewRequest(method, httpServer.URL+path, strings.NewReader(json))
		if err != nil {
			return nil, err
		}

		if len(contentType) == 1 {
			req.Header.Add("Content-Type", contentType[0])
		} else {
			req.Header.Add("Content-Type", "application/json")
		}

		return httpServer.Client().Do(req)
	}
}

func createMultipartRequest[T any](t *testing.T, config *cadet.Config, context T) (*cadet.Server[T], multipartRequestFn) {
	t.Helper()

	server := cadet.NewServer(config, context)
	httpServer := httptest.NewServer(server.Handler())

	return server, func(method, path string, form formData) (*http.Response, error) {
		buffer := &bytes.Buffer{}
		mw := multipart.NewWriter(buffer)

		for k, v := range form {
			mw.WriteField(k, v)
		}

		if err := mw.Close(); err != nil {
			return nil, err
		}

		req, err := http.NewRequest(method, httpServer.URL+path, io.NopCloser(buffer))
		if err != nil {
			return nil, err
		}

		req.ContentLength = int64(buffer.Len())
		req.Header.Add("Content-Type", mw.FormDataContentType())

		return httpServer.Client().Do(req)
	}
}

func TestDefaultPath(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("default", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"default"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}

func TestSlashlessPath(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{Path: "exec"}, "")
	server.Command("slashless", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/exec", `{"name":"slashless"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}

func TestCustomPath(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{Path: "/cmd"}, "")
	server.Command("custom", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/cmd", `{"name":"custom"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}

func TestStrictPath(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{Path: "/"}, "")
	server.Command("strict", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/strict", `{"name":"strict"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusNotFound)
}

func TestUnknownHandler(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("known", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"unknown"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusNotFound)
}

func TestInvalidJSON(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("invalid-json", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `invalid`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusUnprocessableEntity)
}

func TestValidMultipart(t *testing.T) {
	server, req := createMultipartRequest(t, &cadet.Config{}, "")
	server.Command("multipart", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", formData{"command": `{"name":"multipart"}`})

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}

func TestInvalidMultipart(t *testing.T) {
	server, req := createMultipartRequest(t, &cadet.Config{}, "")
	server.Command("multipart", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", formData{"command": `invalid`})

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusUnprocessableEntity)
}

func TestEmptyMultipart(t *testing.T) {
	server, req := createMultipartRequest(t, &cadet.Config{}, "")
	server.Command("multipart", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", formData{"command": ""})

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusUnprocessableEntity)
}

func TestInvalidContentType(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("cmd", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"cmd"}`, "application/fake")

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusUnsupportedMediaType)
}

func TestInvalidMethod(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("cmd", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodGet, "/", `{"name":"cmd"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusMethodNotAllowed)
}

func TestContext(t *testing.T) {
	type context struct {
		Foo string
		Bar string
	}

	server, req := createJSONRequest(t, &cadet.Config{}, &context{"foo", "bar"})
	server.Command("cmd", func(r *cadet.Request, ctx *context) cadet.Response {
		assertEqual(t, ctx.Foo, "foo")
		assertEqual(t, ctx.Bar, "bar")
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"cmd"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}

func TestMiddleware(t *testing.T) {
	testMiddleware := func(header, greeting string) cadet.Middleware {
		return func(h http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(header, greeting)
				h(w, r)
			}
		}
	}

	server, req := createJSONRequest(t, &cadet.Config{Path: "/"}, "", testMiddleware("X-Test1", "Hey"), testMiddleware("X-Test2", "Yo"))
	server.Command("cmd", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"cmd"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
	assertEqual(t, resp.Header.Get("X-Test1"), "Hey")
	assertEqual(t, resp.Header.Get("X-Test2"), "Yo")
}

func TestMiddlewareOrder(t *testing.T) {
	log := ""

	orderMiddleware := func(value string) cadet.Middleware {
		return func(h http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				log += value
				h(w, r)
			}
		}
	}

	middleware := []cadet.Middleware{
		orderMiddleware("1"), orderMiddleware("2"), orderMiddleware("3"),
	}

	server, req := createJSONRequest(t, &cadet.Config{Path: "/"}, "", middleware...)
	server.Command("cmd", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"cmd"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
	assertEqual(t, log, "123")
}

func TestCORS(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{Path: "/"}, "", cadet.CORS("*"))
	server.Command("strict", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodOptions, "/", `{"name":"strict"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Origin"), "*")
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Credentials"), "true")
	assertEqual(t, resp.Header.Get("Access-Control-Expose-Headers"), "X-Auth-Token")
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Headers"), "Origin, Content-Type, Accept, Authorization, X-Requested-With")
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Methods"), "GET,POST,PUT,DELETE,OPTIONS")
}

func TestCORSIgnoredUnlessOptions(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{Path: "/"}, "", cadet.CORS("*"))
	server.Command("ignore", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"ignore"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Origin"), "")
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Credentials"), "")
	assertEqual(t, resp.Header.Get("Access-Control-Expose-Headers"), "")
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Headers"), "")
	assertEqual(t, resp.Header.Get("Access-Control-Allow-Methods"), "")
}

func TestMounting(t *testing.T) {
	mux := http.NewServeMux()

	server := &http.Server{
		Addr:         ":9500",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	cadetServer := cadet.NewServer(&cadet.Config{}, "")
	mux.Handle("/cadet", cadetServer)

	cadetServer.Command("cmd", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusAccepted)
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		listener, err := net.Listen("tcp", ":9500")
		if err != nil {
			return
		}

		wg.Done()

		if err := server.Serve(listener); err != nil {
			return
		}
	}()

	wg.Wait()

	resp, err := http.Post("http://localhost:9500", "application/json", strings.NewReader(""))
	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusCreated)

	resp, err = http.Post("http://localhost:9500/cadet", "application/json", strings.NewReader(`{"name":"cmd"}`))
	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusAccepted)

	server.Shutdown(context.Background())
}

func TestMultipleCommands(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{Path: "/"}, "")

	cmd1 := func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusOK)
	}

	cmd2 := func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusAccepted)
	}

	cmd3 := func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusNonAuthoritativeInfo)
	}

	server.Commands(
		"cmd1", cmd1,
		"cmd2", cmd2,
		"cmd3", cmd3,
	)

	resp, err := req(http.MethodPost, "/", `{"name":"cmd1"}`)
	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)

	resp, err = req(http.MethodPost, "/", `{"name":"cmd2"}`)
	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusAccepted)

	resp, err = req(http.MethodPost, "/", `{"name":"cmd3"}`)
	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusNonAuthoritativeInfo)
}

func TestAddCommands(t *testing.T) {
	server, _ := createJSONRequest(t, &cadet.Config{Path: "/"}, "")

	handler := func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Status(http.StatusNonAuthoritativeInfo)
	}

	err := server.Commands()
	assertError(t, err)

	err = server.Commands("cmd")
	assertError(t, err)

	err = server.Commands(handler, handler)
	assertError(t, err)

	err = server.Commands("cmd", "handler")
	assertError(t, err)

	err = server.Commands("cmd1", handler, "cmd2")
	assertError(t, err)
}

func TestRequestReadCommand(t *testing.T) {
	type Data struct {
		Foo string `json:"foo"`
		Bar string `json:"bar"`
	}

	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("cmd", func(r *cadet.Request, ctx string) cadet.Response {
		data := &Data{}
		err := r.ReadCommand(data)

		assertNoError(t, err)
		assertEqual(t, data.Foo, "abc")
		assertEqual(t, data.Bar, "def")

		return cadet.Status(http.StatusOK)
	})

	resp, err := req(http.MethodPost, "/", `{"name":"cmd","data":{"foo":"abc","bar":"def"}}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}

func TestJSONResponse(t *testing.T) {
	type response struct {
		Field string `json:"field"`
	}

	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("json", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.JSON(&response{"value"})
	})

	resp, err := req(http.MethodPost, "/", `{"name":"json"}`)

	assertNoError(t, err)
	assertEqual(t, resp.Header.Get("Content-Type"), "application/json; charset=utf-8")
	assertEqual(t, resp.StatusCode, http.StatusOK)

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)

	assertNoError(t, err)
	assertEqual(t, strings.TrimSpace(string(data)), `{"field":"value"}`)
}

func TestErrorResponse(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("error", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Error(http.StatusInternalServerError, "oops")
	})

	resp, err := req(http.MethodPost, "/", `{"name":"error"}`)

	assertNoError(t, err)
	assertEqual(t, resp.Header.Get("Content-Type"), "application/json; charset=utf-8")
	assertEqual(t, resp.StatusCode, http.StatusInternalServerError)

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)

	assertNoError(t, err)
	assertEqual(t, strings.TrimSpace(string(data)), `{"error":"oops"}`)
}

func TestTextResponse(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("text", func(r *cadet.Request, ctx string) cadet.Response {
		return cadet.Text("text")
	})

	resp, err := req(http.MethodPost, "/", `{"name":"text"}`)

	assertNoError(t, err)
	assertEqual(t, resp.Header.Get("Content-Type"), "text/plain; charset=utf-8")
	assertEqual(t, resp.StatusCode, http.StatusOK)

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)

	assertNoError(t, err)
	assertEqual(t, strings.TrimSpace(string(data)), `text`)
}

func TestNilResponse(t *testing.T) {
	server, req := createJSONRequest(t, &cadet.Config{}, "")
	server.Command("nil", func(r *cadet.Request, ctx string) cadet.Response {
		return nil
	})

	resp, err := req(http.MethodPost, "/", `{"name":"nil"}`)

	assertNoError(t, err)
	assertEqual(t, resp.StatusCode, http.StatusOK)
}
