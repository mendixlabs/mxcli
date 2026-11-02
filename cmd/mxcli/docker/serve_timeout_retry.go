// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"io"
	"net"
)

// buildRetryingServeTimeout runs a boot's build, and when the serve process did
// not answer in time, restarts it and builds once more.
//
// A timed-out request is not a verdict on the model: a `mxbuild --serve` that
// stops answering is process state, and a fresh process builds the same project
// fine — reported as `mxcli test --local` ending in "serve build request ...
// context deadline exceeded", with the next run passing unchanged
// (ako/mxcli#1834). Once, not until it works: a second timeout is reported. Any
// other outcome is returned as it is — a failed build is the model's answer,
// and a refused connection is a dead process a restart would only paper over.
func buildRetryingServeTimeout(serve serveProcess, req BuildRequest, w io.Writer) (*BuildResult, error) {
	res, err := serve.Build(req)
	if err == nil || !isTimeout(err) {
		return res, err
	}
	fmt.Fprintf(w, "  mxbuild --serve did not answer the build request (%v); restarting it and building once more...\n", err)
	if rerr := serve.Restart(); rerr != nil {
		return nil, fmt.Errorf("%w (restarting mxbuild --serve to retry failed: %v)", err, rerr)
	}
	return serve.Build(req)
}

// isTimeout reports whether err is a request that ran out of time, as net/http
// reports an elapsed Client.Timeout.
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
