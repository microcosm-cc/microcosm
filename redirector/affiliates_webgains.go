package redirector

import (
	"net/url"
	"strconv"

	"github.com/golang/glog"

	"github.com/microcosm-cc/microcosm/models"
)

const webGainsAffiliateID string = "104653"

var webgainsDomainParts = []string{
	"awcycles",
	"cyclesurgery",
	"runnersneed",
	"webgains",
}

type webgainsLink struct {
	Link models.Link
}

func (m *webgainsLink) getDestination() (bool, string) {

	// Hijack an existing affiliate link
	if m.Link.Domain == "track.webgains.com" {
		u, err := url.Parse(m.Link.URL)
		if err != nil {
			glog.Errorf("url.Parse(`%s`) %+v", m.Link.URL, err)
			return false, m.Link.URL
		}

		q := u.Query()
		q.Del("wgcampaignid")
		q.Add("wgcampaignid", webGainsAffiliateID)
		u.RawQuery = q.Encode()

		return true, u.String()
	}

	// Fetch a program ID based on domain
	var programID int
	switch m.Link.Domain {
	case "www.awcycles.co.uk":
		programID = 2730
	case "www.cyclesurgery.com":
		programID = 5505
	case "www.runnersneed.com":
		programID = 5503
	case "www.westbrookcycles.co.uk":
		programID = 7793
	default:
		return false, m.Link.URL
	}

	// Create our affiliate link
	u, _ := url.Parse("http://track.webgains.com/click.html")
	q := u.Query()
	q.Add("wgcampaignid", webGainsAffiliateID)
	q.Add("wgprogramid", strconv.Itoa(programID))
	q.Add("wgtarget", m.Link.URL)
	u.RawQuery = q.Encode()

	return true, u.String()
}
