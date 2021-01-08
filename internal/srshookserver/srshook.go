package srshookserver

import (
	"net/http"
	"playground/internal/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type srsHook struct{}

func newSrsHook() *srsHook {
	return &srsHook{}
}

func (h *srsHook) traceID(c *gin.Context) {
	traceId := uuid.New().String()
	c.Set("trace-id", traceId)

	c.Header("X-Request-Id", traceId)
}

func (h *srsHook) ping(c *gin.Context) {
	c.JSON(http.StatusOK, errno.ErrOK.WithData("pong"))
}

func (h *srsHook) publish(c *gin.Context) {
	c.JSON(http.StatusOK, errno.ErrServer)
}
