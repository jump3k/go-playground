package logging

import (
	"io"
	"os"
	"strings"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
)

type LogConfig struct {
	LogPath         string
	RotationTime    time.Duration
	ReverseDays     int
	Level           string
	Format          string
	SetRepoerCaller bool
	UseStderr       bool
}

func (lc *LogConfig) NewLogger() (*logrus.Logger, error) {
	var out io.Writer
	if lc.UseStderr {
		out = os.Stderr
	} else {
		logWriter, err := rotatelogs.New(
			lc.LogPath+"_%Y%m%d",
			rotatelogs.WithLinkName(lc.LogPath),
			rotatelogs.WithRotationTime(lc.RotationTime),
			rotatelogs.WithMaxAge(time.Duration(lc.ReverseDays)*24*time.Hour),
		)
		if err != nil {
			return nil, err
		}

		out = logWriter
	}

	logger := logrus.New()
	logger.SetOutput(out)

	switch strings.ToLower(lc.Format) {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	case "text":
		fallthrough
	default:
		logger.SetFormatter(&logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		})
	}

	if level, err := logrus.ParseLevel(lc.Level); err != nil {
		logger.SetLevel(logrus.ErrorLevel)
	} else {
		logger.SetLevel(level)
	}

	if lc.SetRepoerCaller {
		logger.SetReportCaller(true)
	}

	return logger, nil
}
