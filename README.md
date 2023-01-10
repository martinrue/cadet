# cadet

[![cadet tests](https://github.com/martinrue/cadet/actions/workflows/test.yml/badge.svg)](https://github.com/martinrue/cadet/actions/workflows/test.yml)

cadet is a library for creating simple HTTP-RPC servers in Go.

```go
package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/martinrue/cadet"
)

type EchoMaker struct{}

func (e *EchoMaker) Echo(text string) string {
	return fmt.Sprintf("%s %s %s", text, text, text)
}

type EchoCommand struct {
	Text string `json:"text"`
}

type EchoResponse struct {
	Echo string `json:"echo"`
}

func EchoHandler(r *cadet.Request, em *EchoMaker) cadet.Response {
	cmd := &EchoCommand{}

	if err := r.ReadCommand(&cmd); err != nil {
		return cadet.Status(http.StatusUnprocessableEntity)
	}

	echo := em.Echo(cmd.Text)
	return cadet.JSON(&EchoResponse{echo})
}

func main() {
	server := cadet.NewServer(&cadet.Config{Bind: ":1234"}, &EchoMaker{})

	err := server.Commands(
		"echo", EchoHandler,
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to register commands: %v", err)
		os.Exit(1)
	}

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start server: %v", err)
		os.Exit(1)
	}
}
```

```
> curl -X POST -H "Content-Type: application/json" -d '{"name":"echo","data":{"text":"Yo"}}' http://localhost:1234
{"echo":"Yo Yo Yo"}
```

## Features

### Small API

The API surface of cadet is very small. Once you've created a server, registering commands is as easy as calling `server.Commands()` with pairs of command names and handler functions.

```go
func main() {
	server := cadet.NewServer(/* ... */)

	server.Commands(
		"register", RegisterHandler,
		"sign-in", SignInHandler,
		"delete-account", DeleteAccountHandler,
	)

	server.Start();
}
```

### Type-safe dependencies

Pass in custom dependencies when creating your server and when a command handler is called it'll have full access to those dependencies in a type-safe way.

```go
func RegisterHandler(r *cadet.Request, db *Database) cadet.Response {
	db.CreateUser("name", "email")
	return cadet.Status(http.StatusOK)
}

func main() {
	db := &Database{}
	server := cadet.NewServer(&cadet.Config{Bind: ":1234"}, db)

	server.Commands(
		"register", RegisterHandler,
	)

	// ...
}
```

### Command parsing

Each handler is passed a `cadet.Request` object to help you parse optional command data. The request also contains the underlying `*http.Request` and `http.ResponseWriter`, allowing you to do anything you'd do in a normal http `HandlerFunc`.

```go
type EchoCommand struct {
	Text string `json:"text"`
}

func EchoHandler(r *cadet.Request, db *Database) cadet.Response {
	cmd := &EchoCommand{}

	if err := r.ReadCommand(&cmd); err != nil {
		return cadet.Status(http.StatusUnprocessableEntity)
	}

	r.RawResponse.Header().Add("X-Echo", echo)
	return cadet.Status(http.StatusOK)
}
```

### Response types

Handlers must return a `cadet.Response`, which captures a value that cadet will serialise for you and send back with the correct content type and encoding.

```go
type Forecast struct {
	Degrees int    `json:"degrees"`
	Text    string `json:"text"`
}

func WeatherHandler(r *cadet.Request, db *Database) cadet.Response {
	forecast := &Forecast{
		Degrees: -2,
		Text:    "Do bundle up, it's awfully cold outside.",
	}

	return cadet.JSON(forecast)
}
```

In addition to `cadet.JSON()`, handlers can also return `cadet.Text()`, `cadet.Status()` and `cadet.Error()`.

### Multipart handling

To support things like image upload, cadet also supports requests made with a `multipart/form-data` content type. Cadet will parse the JSON message and invoke your handler as normal, giving you a `*cadet.Request`.

Via `cadet.Request.RawRequest` you can access the form data, read files, and perform any custom logic necessary.

```go
func UploadHandler(r *cadet.Request, db *Database) cadet.Response {
	file, header, err := r.RawRequest.FormFile("file")
	if err != nil {
		return cadet.Error(http.StatusUnprocessableEntity, "no file attached")
	}

	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return cadet.Error(http.StatusInternalServerError, "read failed")
	}

	if err := os.WriteFile(header.Filename, bytes, 0644); err != nil {
		return cadet.Error(http.StatusInternalServerError, "write failed")
	}

	return cadet.Status(http.StatusOK)
}
```

### Middleware

To run code before/afer handlers run, call `server.Use()` to pass in middleware functions.

```go
func withHeader(key, value string) cadet.Middleware {
	return func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add(key, value)
			h(w, r)
		}
	}
}

func main() {
	server := cadet.NewServer(&cadet.Config{Bind: ":1234"}, &Database{})

	server.Use(
		withHeader("X-Server", "cadet"),
		withHeader("X-Server-Version", "0.0.1"),
	)

	err := server.Commands(
		"cmd", Handler,
	)

	// ...
}
```

### Mounting

The cadet server implements the [http.Handler](https://pkg.go.dev/net/http#Handler) interface, allowing it to be easily mounted within an existing http project.

```go
func main() {
	server := cadet.NewServer(&cadet.Config{}, "")

	server.Commands(
		"send-email", SendEmailHandler,
	)

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.Handle("/cadet", server)

	http.ListenAndServe(":9999", nil)
}
```

## Message format

A command is invoked by sending a JSON message (via `POST`) that contains at least a `name` matching a registered command, and optionally `data` containing additional data:

```json
{ "name": "sign-in", "data": { "email": "me@home.com" } }
```

To handle other kinds of incoming data, such as file uploads, cadet also supports `multipart/form-data` requests. In a `multipart/form-data` scenario, cadet expects to find the JSON message as a key named `command`.
