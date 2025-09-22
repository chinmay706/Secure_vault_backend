package testing

import (
	"net/http"
	"securevault-backend/src/internal/app"
)

// NewTestApp creates a test application instance for testing
func NewTestApp() (http.Handler, func() error, error) {
	testApp, err := app.NewTestApp()
	if err != nil {
		return nil, nil, err
	}

	return testApp.Router(), testApp.Close, nil
}