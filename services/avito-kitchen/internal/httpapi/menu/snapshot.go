package menu

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

func (h *Handler) replaceSnapshot(w http.ResponseWriter, r *http.Request) {
	var request snapshotRequest
	if err := common.DecodeJSON(w, r, &request); err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	result, err := h.service.ReplaceSnapshot(
		r.Context(),
		r.PathValue("restaurant_id"),
		snapshotInputFromRequest(request),
	)
	if err != nil {
		var staleError *menuservice.StaleSnapshotError
		switch {
		case errors.Is(err, menuservice.ErrInvalidMenu):
			common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, menuservice.ErrRestaurantNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "RESTAURANT_NOT_FOUND", "Restaurant not found")
		case errors.As(err, &staleError):
			common.WriteJSON(r.Context(), h.logger, w, http.StatusConflict, staleSnapshotResponse{
				Error: staleSnapshotErrorBody{
					Code:    "STALE_SNAPSHOT_UPDATE",
					Message: "Menu snapshot version is older than current version",
					Details: staleSnapshotDetails{
						CurrentSnapshotVersion:  staleError.CurrentVersion,
						ReceivedSnapshotVersion: staleError.ReceivedVersion,
					},
				},
			})
		default:
			h.logger.ErrorContext(r.Context(), "replace menu snapshot", "error", err)
			common.WriteError(
				r.Context(),
				h.logger,
				w,
				http.StatusInternalServerError,
				"INTERNAL_ERROR",
				"Internal server error",
			)
		}
		return
	}

	common.WriteJSON(r.Context(), h.logger, w, http.StatusOK, snapshotResponseFromService(result))
}
