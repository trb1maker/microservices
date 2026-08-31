package logging_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/internal/platform/logging"
)

func TestNew_jsonLoggerDoesNotNeedNetwork(t *testing.T) {
	t.Parallel()

	logger, err := logging.New("info", "json")
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Info("loki-unrelated stdout log")
}
