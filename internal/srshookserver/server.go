package srshookserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type server struct {
	listenAddr string //监听地址
	enginName  string //http驱动名称
	mode       string //运行模式, debug/release/test

	engine interface{} //底层Http驱动, 比如: gin
}

func New(listenAddr string, enginName string, mode string) *server {
	return &server{
		listenAddr: listenAddr,
		enginName:  enginName,
		mode:       mode,
	}
}

func (s *server) Run() error {
	var err error

	switch s.setHttpEngine().(type) {
	case *gin.Engine:
		err = s.setRouterForGin(s.engine.(*gin.Engine)).Run(s.listenAddr)
	default:
		err = errors.New("unsupport")
	}

	if err != nil {
		logrus.Fatal(err)
		return err
	}

	return nil
}

func (s *server) setHttpEngine() interface{} {
	switch s.enginName {
	case "gin":
		gin.SetMode(s.mode)
		s.engine = gin.New()
	default:
		return nil
	}

	return s.engine
}

func (s *server) setRouterForGin(r *gin.Engine) *gin.Engine {
	hook := newSrsHook()
	r.Use(hook.traceID)

	apis := []apiEntry{
		{method: "GET", url: "/ping", handler: hook.ping},
		{method: "POST", url: "/v1/publish", handler: hook.publish},
	}

	for _, api := range apis {
		switch api.method {
		case "GET":
			r.GET(api.url, api.handler)
		case "POST":
			r.POST(api.url, api.handler)
		}
	}

	return r
}

type apiEntry struct {
	method  string               //请求方法
	url     string               //请求url
	handler func(c *gin.Context) //API入口
}
