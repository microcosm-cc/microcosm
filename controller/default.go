package controller

import (
	"net/http"

	h "github.com/microcosm-cc/microcosm/helpers"
	"github.com/microcosm-cc/microcosm/models"
)

// RootHandler is a web handler
func RootHandler(w http.ResponseWriter, r *http.Request) {
	c, status, err := models.MakeContext(r, w)
	if err != nil {
		c.RespondWithErrorDetail(err, status)
		return
	}

	switch c.GetHTTPMethod() {
	case "OPTIONS":
		c.RespondWithOptions([]string{"OPTIONS", "GET"})
		return
	case "GET":
		c.RespondWithData(
			h.LinkArrayType{Links: []h.LinkType{
				h.LinkType{Rel: "api", Href: "/api"},
			}},
		)
		return
	default:
		c.RespondWithStatus(http.StatusMethodNotAllowed)
		return
	}
}

// APIHandler is a web handler
func APIHandler(w http.ResponseWriter, r *http.Request) {
	c, status, err := models.MakeContext(r, w)
	if err != nil {
		c.RespondWithErrorDetail(err, status)
		return
	}

	switch c.GetHTTPMethod() {
	case "OPTIONS":
		c.RespondWithOptions([]string{"OPTIONS", "GET"})
		return
	case "GET":
		c.RespondWithData(
			h.LinkArrayType{Links: []h.LinkType{
				h.LinkType{Rel: "v1", Href: "/api/v1"},
			}},
		)
		return
	default:
		c.RespondWithStatus(http.StatusMethodNotAllowed)
		return
	}
}

// V1Handler is a web handler
func V1Handler(w http.ResponseWriter, r *http.Request) {
	c, status, err := models.MakeContext(r, w)
	if err != nil {
		c.RespondWithErrorDetail(err, status)
		return
	}

	switch c.GetHTTPMethod() {
	case "OPTIONS":
		c.RespondWithOptions([]string{"OPTIONS", "GET"})
		return
	case "GET":
		c.RespondWithData(
			h.LinkArrayType{Links: []h.LinkType{
				h.GetLink("activity", "", h.ItemTypeActivity, 0),
				h.GetLink("auth", "", h.ItemTypeAuth, 0),
				h.GetLink("comment", "", h.ItemTypeComment, 0),
				h.GetLink("conversation", "", h.ItemTypeConversation, 0),
				h.GetLink("event", "", h.ItemTypeEvent, 0),
				h.GetLink("microcosm", "", h.ItemTypeMicrocosm, 0),
				h.GetLink("poll", "", h.ItemTypePoll, 0),
				h.GetLink("profile", "", h.ItemTypeProfile, 0),
				h.GetLink("question", "", h.ItemTypeQuestion, 0),
				h.LinkType{Rel: "site", Href: "/api/v1/site"},
				h.GetLink("update", "", h.ItemTypeUpdate, 0),
				h.GetLink("watcher", "", h.ItemTypeWatcher, 0),
				h.GetLink("whoami", "", h.ItemTypeWhoAmI, 0),
			}},
		)
		return
	default:
		c.RespondWithStatus(http.StatusMethodNotAllowed)
		return
	}
}
