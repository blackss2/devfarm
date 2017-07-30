package main

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/blackss2/devfarm/pkg/builder"
	"github.com/blackss2/devfarm/pkg/runner"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/satori/go.uuid"
	"golang.org/x/net/websocket"
)

func main() {
	e := echo.New()
	e.Use(middleware.Recover())

	RunContextHash := make(map[string]*RunContext)

	g := e.Group("/api")
	g.POST("/spaces", func(c echo.Context) error {
		data, err := ioutil.ReadAll(c.Request().Body)
		if err != nil {
			panic(err)
		}

		binary, err := builder.BuildFromSourceZip(data)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		ctx, cancel := context.WithCancel(context.Background())
		rc := NewRunContext(ctx, cancel)
		go func() {
			defer rc.Close()

			err := runner.RunFromBinaryZip(ctx, binary, rc.stdin, rc.stdout, rc.stderr, rc.portchan)
			if err != nil {
				panic(err)
			}
		}()
		Id := uuid.NewV1().String()
		RunContextHash[Id] = rc

		return c.String(http.StatusOK, Id)
	})
	g.GET("/spaces/:sid/stdin", func(c echo.Context) error {
		sid := c.Param("sid")
		rc, has := RunContextHash[sid]
		if !has {
			panic("not exist sid")
		}

		websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			defer rc.Close()
			defer delete(RunContextHash, sid)
			for {
				msg := ""
				err := websocket.Message.Receive(ws, &msg)
				if err != nil {
					return
				}

				_, err = rc.stdin.Write([]byte(msg))
				if err != nil {
					return
				}
			}
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})
	g.GET("/spaces/:sid/stdout", func(c echo.Context) error {
		sid := c.Param("sid")
		rc, has := RunContextHash[sid]
		if !has {
			panic("not exist sid")
		}

		websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			defer rc.Close()
			defer delete(RunContextHash, sid)

			msg := make([]byte, 1000)
			for {
				n, err := rc.stdout.Read(msg)
				if err != nil {
					return
				}
				err = websocket.Message.Send(ws, string(msg[:n]))
				if err != nil {
					return
				}
			}
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})
	g.GET("/spaces/:sid/stderr", func(c echo.Context) error {
		sid := c.Param("sid")
		rc, has := RunContextHash[sid]
		if !has {
			panic("not exist sid")
		}

		websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			defer rc.Close()
			defer delete(RunContextHash, sid)

			msg := make([]byte, 1000)
			for {
				n, err := rc.stderr.Read(msg)
				if err != nil {
					return
				}
				err = websocket.Message.Send(ws, string(msg[:n]))
				if err != nil {
					return
				}
			}
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})
	g.GET("/spaces/:sid/portchan", func(c echo.Context) error {
		sid := c.Param("sid")
		rc, has := RunContextHash[sid]
		if !has {
			panic("not exist sid")
		}

		websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			defer rc.Close()
			defer delete(RunContextHash, sid)

			msg := make([]byte, 1000)
			for {
				n, err := rc.portchan.Read(msg)
				if err != nil {
					return
				}
				err = websocket.Message.Send(ws, string(msg[:n]))
				if err != nil {
					return
				}
			}
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})
	e.Start(":80")
}

var (
	ErrChanClosed = errors.New("chan closed")
)

type ChanReadWriter struct {
	sync.Mutex
	waitChan chan struct{}
	done     chan struct{}
	buffer   bytes.Buffer
	isOpen   bool
}

func NewChanReadWriter() *ChanReadWriter {
	cr := &ChanReadWriter{
		waitChan: make(chan struct{}),
		done:     make(chan struct{}),
		isOpen:   true,
	}
	return cr
}

func (cr *ChanReadWriter) Read(bs []byte) (int, error) {
	select {
	case <-cr.done:
		return 0, ErrChanClosed
	case <-cr.waitChan:
		cr.Lock()
		defer cr.Unlock()
		if cr.buffer.Len() == 0 {
			return 0, nil
		}
		if len(bs) > cr.buffer.Len() {
			cbs := cr.buffer.Bytes()
			for i, b := range cbs {
				bs[i] = b
			}
			cr.buffer.Reset()
			return len(cbs), nil
		} else {
			cbs := cr.buffer.Next(len(bs))
			for i, b := range cbs {
				bs[i] = b
			}
			go func() {
				cr.waitChan <- struct{}{}
			}()
			return len(cbs), nil
		}
	}
}

func (cr *ChanReadWriter) Write(bs []byte) (int, error) {
	cr.Lock()
	defer func() {
		cr.Unlock()
		if cr.isOpen {
			cr.waitChan <- struct{}{}
		}
	}()
	n, err := cr.buffer.Write(bs)
	if err != nil {
		return 0, err
	}
	return n, err
}

func (cr *ChanReadWriter) Close() {
	if cr.isOpen {
		close(cr.waitChan)
		close(cr.done)
		cr.isOpen = false
	}
}

type RunContext struct {
	stdin    *ChanReadWriter
	stdout   *ChanReadWriter
	stderr   *ChanReadWriter
	portchan *ChanReadWriter
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewRunContext(ctx context.Context, cancel context.CancelFunc) *RunContext {
	rc := &RunContext{
		stdin:    NewChanReadWriter(),
		stdout:   NewChanReadWriter(),
		stderr:   NewChanReadWriter(),
		portchan: NewChanReadWriter(),
		ctx:      ctx,
		cancel:   cancel,
	}
	return rc
}

func (rc *RunContext) Close() {
	rc.stdin.Close()
	rc.stdout.Close()
	rc.stderr.Close()
	rc.cancel()
}
