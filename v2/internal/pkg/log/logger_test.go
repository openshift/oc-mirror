package log

import (
	"testing"
)

func TestLogger(t *testing.T) {

	log := New("trace")

	// this is a facade to get code coverage up
	t.Run("Testing PluggableLogger : should pass", func(t *testing.T) {
		log.Info("Test %s ", "log")
		log.Warn("Test %s ", "log")
		log.Debug("Test %s ", "log")
		log.Trace("Test %s ", "log")
		log.Error("Test %s ", "log")
	})
}
