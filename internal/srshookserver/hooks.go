package srshookserver

import (
	"net/http"
	"playground/internal/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type hook struct{}

func newHook() *hook {
	return &hook{}
}

func (h *hook) traceID(c *gin.Context) {
	traceId := uuid.New().String()
	c.Set("trace-id", traceId)

	c.Header("X-Request-Id", traceId)
}

func (h *hook) ping(c *gin.Context) {
	c.JSON(http.StatusOK, errno.ErrOK.WithData("pong").WithID(c.GetString("trace-id")))
}

func (h *hook) publish(c *gin.Context) {
	c.JSON(http.StatusOK, errno.ErrServer.WithID(c.GetString("trace-id")))
}
