package source

import (
	"os"
	"testing"

	"github.com/kurenn/slack-tui/internal/testenv"
)

func TestMain(m *testing.M) { os.Exit(testenv.Pin(m)) }
