package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClusterOperationErrorDistinguishesRejectionFromUnknown(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"invalid input", fmt.Errorf("%w: invalid node address", cluster.ErrInvalid), http.StatusBadRequest},
		{"revision conflict", cluster.ErrConflict, http.StatusConflict},
		{"unauthorized", cluster.ErrForbidden, http.StatusForbidden},
		{"missing operation", cluster.ErrNotFound, http.StatusNotFound},
		{"unsafe retirement", cluster.ErrUnsafeExit, http.StatusConflict},
		{"commit unknown", errors.New("fixture: commit acknowledgement lost"), http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			clusterError(ctx, test.err)
			require.Equal(t, test.status, response.Code)
			if test.status == http.StatusServiceUnavailable {
				require.NotContains(t, response.Body.String(), test.err.Error())
			}
		})
	}
}
