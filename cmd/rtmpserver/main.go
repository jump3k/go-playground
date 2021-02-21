package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"playground/internal/logging"
	"playground/pkg/rtmp"

	"github.com/sirupsen/logrus"
)

func initLogger(config *rtmp.Config) (*logrus.Logger, error) {
	return (&logging.LogConfig{
		LogPath:         "logs/error.log",
		RotationTime:    24 * time.Hour,
		ReverseDays:     7,
		Level:           "INFO",
		Format:          "text",
		UseStderr:       true,
		SetRepoerCaller: true,
	}).NewLogger()
}

func main() {
	go func() {
		_ = http.ListenAndServe(":6060", nil) //pprof
	}()

	go func() {
		config := &rtmp.Config{} //TODO

		logger, err := initLogger(config)
		if err != nil {
			panic(err)
		}
		config.Logger = logger

		if err := rtmp.ListenAndServe("tcp", ":1935", config); err != nil {
			logrus.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
