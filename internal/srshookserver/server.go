package srshookserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type server struct {
	e          interface{} //底层Http驱动, 比如: gin
	listenAddr string
}

func NewSrsHookServer(listenAddr string) *server {
	return &server{
		listenAddr: listenAddr,
	}
}

func (s *server) Run() error {
	var err error

	switch s.setupRouter().(type) {
	case *gin.Engine:
		err = s.e.(*gin.Engine).Run(s.listenAddr)
	default:
		err = errors.New("unspport")
	}

	if err != nil {
		logrus.Error(err)
		return err
	}

	return nil
}

func (s *server) setupRouter() interface{} {
	r := gin.New()
	s.e = r

	r.Use(newHook().traceID)
	for _, api := range apis {
		switch api.method {
		case "GET":
			r.GET(api.url, api.hook)
		case "POST":
			r.POST(api.url, api.hook)
		}
	}

	return s.e
}

type apiEntry struct {
	method string               //请求方法
	url    string               //请求url
	hook   func(c *gin.Context) //API入口
}

var apis []apiEntry = []apiEntry{
	{method: "GET", url: "/ping", hook: newHook().ping},
	{method: "POST", url: "/v1/publish", hook: newHook().publish},
}
