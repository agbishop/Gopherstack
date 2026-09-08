package azurearm_test

import (
	"testing"

	"github.com/blackbirdworks/gopherstack/pkgs/testleak"
)

// TestMain asserts azurearm tests leave no goroutines running, guarding the
// dedicated HTTPS listener goroutine (Handler.StartWorker) against leaks --
// every test that calls StartWorker must also call Shutdown (directly or via
// t.Cleanup), or this will fail the whole package's test run.
func TestMain(m *testing.M) {
	testleak.VerifyTestMain(m)
}
