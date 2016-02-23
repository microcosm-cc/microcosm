package models

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"

	h "github.com/microcosm-cc/microcosm/helpers"
)

// SummaryContainer stores a summary of an item
type SummaryContainer struct {
	ItemTypeID int64       `json:"-"`
	ItemType   string      `json:"itemType"`
	ItemID     int64       `json:"-"`
	Summary    interface{} `json:"item"`
	Valid      bool        `json:"-"`
}

// SummaryContainerRequest allows a request to be passed by a channel and the
// error, status and sequence of responses to be encapsulated
type SummaryContainerRequest struct {
	Item   SummaryContainer
	Err    error
	Status int
	Seq    int
}

// SummaryContainerRequestsBySeq is an array of requests
type SummaryContainerRequestsBySeq []SummaryContainerRequest

// Len gives the length of the array
func (v SummaryContainerRequestsBySeq) Len() int {
	return len(v)
}

// Swap allows re-ordering of array elements by swapping them around
func (v SummaryContainerRequestsBySeq) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// Less determines whether the given items are above or below each other in the
// array
func (v SummaryContainerRequestsBySeq) Less(i, j int) bool {
	return v[i].Seq < v[j].Seq
}

// HandleSummaryContainerRequest wraps GetSummaryContainer
func HandleSummaryContainerRequest(
	siteID int64,
	itemTypeID int64,
	itemID int64,
	profileID int64,
	seq int,
	out chan<- SummaryContainerRequest,
) {

	item, status, err := GetSummaryContainer(
		siteID,
		itemTypeID,
		itemID,
		profileID,
	)

	response := SummaryContainerRequest{
		Item:   item,
		Status: status,
		Err:    err,
		Seq:    seq,
	}
	out <- response
}

// GetSummaryContainer wraps GetSummary
func GetSummaryContainer(
	siteID int64,
	itemTypeID int64,
	itemID int64,
	profileID int64,
) (
	SummaryContainer,
	int,
	error,
) {

	summary, status, err := GetSummary(siteID, itemTypeID, itemID, profileID)
	if err != nil {
		return SummaryContainer{}, status, err
	}

	item := SummaryContainer{}
	item.ItemTypeID = itemTypeID

	itemType, _ := h.GetMapStringFromInt(h.ItemTypes, itemTypeID)
	item.ItemType = itemType

	item.ItemID = itemID
	item.Summary = summary
	item.Valid = true

	return item, http.StatusOK, nil
}

// GetSummary fetches the smallest and most cacheable representation of a thing,
// usually the result of Get<Item>Summary
//
// In this context, the 4th arg (profileId) is for the person asking for the summary
// That is needed for strongly permission items such as huddles.
func GetSummary(
	siteID int64,
	itemTypeID int64,
	itemID int64,
	profileID int64,
) (
	interface{},
	int,
	error,
) {

	if itemID == 0 {
		glog.Errorf(
			"GetSummary(%d, %d, %d, %d) Item not found",
			siteID,
			itemTypeID,
			itemID,
			profileID,
		)
		return nil, http.StatusNotFound, fmt.Errorf("Item not found")
	}

	switch itemTypeID {

	case h.ItemTypes[h.ItemTypeAlbum]:

	case h.ItemTypes[h.ItemTypeArticle]:

	case h.ItemTypes[h.ItemTypeAttendee]:
		summary, status, err := GetProfileSummary(siteID, itemID)
		if err != nil {
			glog.Errorf(
				"GetProfileSummary(%d, %d) %+v",
				siteID,
				itemID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeClassified]:

	case h.ItemTypes[h.ItemTypeComment]:
		summary, status, err := GetCommentSummary(siteID, itemID)
		if err != nil {
			glog.Errorf(
				"GetCommentSummary(%d, %d) %+v",
				siteID,
				itemID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeConversation]:
		summary, status, err := GetConversationSummary(
			siteID,
			itemID,
			profileID,
		)
		if err != nil {
			glog.Errorf(
				"GetConversationSummary(%d, %d, %d) %+v",
				siteID,
				itemID,
				profileID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeEvent]:
		summary, status, err := GetEventSummary(siteID, itemID, profileID)
		if err != nil {
			glog.Errorf(
				"GetEventSummary(%d, %d, %d) %+v",
				siteID,
				itemID,
				profileID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeHuddle]:
		summary, status, err := GetHuddleSummary(siteID, profileID, itemID)
		if err != nil {
			glog.Errorf(
				"GetHuddleSummary(%d, %d, %d) %+v",
				siteID,
				profileID,
				itemID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeMicrocosm]:
		summary, status, err := GetMicrocosmSummary(
			siteID,
			itemID,
			profileID,
		)
		if err != nil {
			glog.Errorf(
				"GetMicrocosmSummary(%d, %d, %d) %+v",
				siteID,
				itemID,
				profileID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypePoll]:
		summary, status, err := GetPollSummary(siteID, itemID, profileID)
		if err != nil {
			glog.Errorf(
				"GetPollSummary(%d, %d, %d) %+v",
				siteID,
				itemID,
				profileID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeProfile]:
		summary, status, err := GetProfileSummary(siteID, itemID)
		if err != nil && status != http.StatusNotFound {
			glog.Errorf(
				"GetProfileSummary(%d, %d) %+v",
				siteID,
				itemID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeQuestion]:
		summary, status, err := GetQuestionSummary(
			siteID,
			itemID,
			profileID,
		)
		if err != nil {
			glog.Errorf(
				"GetQuestionSummary(%d, %d, %d) %+v",
				siteID,
				itemID,
				profileID,
				err,
			)
		}
		return summary, status, err

	case h.ItemTypes[h.ItemTypeSite]:
		summary, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("GetSite(%d) %+v", siteID, err)
		}
		return summary, status, err

	default:

	}

	return nil, http.StatusInternalServerError,
		fmt.Errorf("GetSummary() not yet implemented")
}

// GetTitle fetches a title of a thing, or something that can be used as a
// title. Will not trim or otherwise shorten the title. In the case of profile
// this returns the profileName, etc.
func GetTitle(
	siteID int64,
	itemTypeID int64,
	itemID int64,
	profileID int64,
) (
	string,
	int,
	error,
) {

	switch itemTypeID {
	case h.ItemTypes[h.ItemTypeMicrocosm]:
		return GetMicrocosmTitle(itemID), http.StatusOK, nil
	case h.ItemTypes[h.ItemTypeHuddle]:
		return GetHuddleTitle(itemID), http.StatusOK, nil
	default:
	}

	summary, status, err := GetSummary(siteID, itemTypeID, itemID, profileID)
	if err != nil {
		glog.Errorf(
			"GetSummary(%d, %d, %d, %d) %+v",
			siteID,
			itemTypeID,
			itemID,
			profileID,
			err,
		)
		return "", status, err
	}

	switch summary.(type) {
	case ConversationSummaryType:
		return summary.(ConversationSummaryType).Title, http.StatusOK, nil

	case EventSummaryType:
		return summary.(EventSummaryType).Title, http.StatusOK, nil

	case PollSummaryType:
		return summary.(PollSummaryType).Title, http.StatusOK, nil

	case ProfileSummaryType:
		return summary.(ProfileSummaryType).ProfileName, http.StatusOK, nil

	case SiteType:
		return summary.(SiteType).Title, http.StatusOK, nil

	default:
	}

	return "", http.StatusNotImplemented,
		fmt.Errorf("GetTitle is not implemented for this item")
}
