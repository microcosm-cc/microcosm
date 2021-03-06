package controller

import (
	"net/http"

	"github.com/microcosm-cc/microcosm/models"
)

// MicrocosmsTreeHandler is a web handler
func MicrocosmsTreeHandler(w http.ResponseWriter, r *http.Request) {
	c, status, err := models.MakeContext(r, w)
	if err != nil {
		c.RespondWithErrorDetail(err, status)
		return
	}

	ctl := MicrocosmsTreeController{}

	switch c.GetHTTPMethod() {
	case "OPTIONS":
		c.RespondWithOptions([]string{"OPTIONS", "POST", "HEAD", "GET"})
		return
	case "HEAD":
		ctl.ReadMany(c)
	case "GET":
		ctl.ReadMany(c)
	default:
		c.RespondWithStatus(http.StatusMethodNotAllowed)
		return
	}
}

// MicrocosmsTreeController is a web controller
type MicrocosmsTreeController struct{}

// ReadMany handles GET
func (ctl *MicrocosmsTreeController) ReadMany(c *models.Context) {
	// Get Microcosm Tree
	m, status, err := models.GetMicrocosmTree(
		c.Site.ID,
		c.Auth.ProfileID,
	)
	if err != nil {
		c.RespondWithErrorDetail(err, status)
		return
	}

	c.RespondWithData(m)
}
