package handlers

import (
	"mydal/internal/service"
	"mydal/internal/testutil"
)

// newTestTrackService wires the concrete service the track handler holds
// around a fake repository, which is the seam the handler tests use.
func newTestTrackService(repo service.TrackRepository) *service.TrackService {
	return service.NewTrackService(repo, nil, testutil.Quiet())
}
