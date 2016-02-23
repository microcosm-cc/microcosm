package models

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"

	"github.com/golang/glog"
	"github.com/lib/pq"

	h "github.com/microcosm-cc/microcosm/helpers"
)

// ItemParent describes the ancestor microcosms this item belongs to
type ItemParent struct {
	MicrocosmID int64                `json:"microcosmId"`
	Breadcrumb  *[]MicrocosmLinkType `json:"breadcrumb,omitempty"`
}

// Item is a set of minimal and common fields used by items that exist on the
// site, usually things that exist within a microcosm
type Item struct {
	ItemParent

	ItemTypeID int64  `json:"-"`
	ItemType   string `json:"itemType"`

	ID    int64  `json:"id"`
	Title string `json:"title"`

	CommentCount int64 `json:"totalComments"`
	ViewCount    int64 `json:"totalViews"`

	LastCommentIDNullable        sql.NullInt64 `json:"-"`
	LastCommentID                int64         `json:"lastCommentId,omitempty"`
	LastCommentCreatedByNullable sql.NullInt64 `json:"-"`
	LastCommentCreatedBy         interface{}   `json:"lastCommentCreatedBy,omitempty"`
	LastCommentCreatedNullable   pq.NullTime   `json:"-"`
	LastCommentCreated           string        `json:"lastCommentCreated,omitempty"`

	Meta h.DefaultMetaType `json:"meta"`
}

// ItemRequest is an envelope to request an item via a channel
type ItemRequest struct {
	Item   Item
	Err    error
	Status int
	Seq    int
}

// ItemRequestBySeq it a sorted collection of ItemRequests
type ItemRequestBySeq []ItemRequest

// Len returns the length of the collection
func (v ItemRequestBySeq) Len() int { return len(v) }

// Swap exchanges two adjacent items in the collection
func (v ItemRequestBySeq) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

// Less determines whether an item is greater in sequence than another
func (v ItemRequestBySeq) Less(i, j int) bool { return v[i].Seq < v[j].Seq }

// ItemSummary is used by all things that can be a child of a microcosm
type ItemSummary struct {
	// Common Fields
	ID    int64  `json:"id"`
	Title string `json:"title"`

	ItemParent
}

// ItemSummaryMeta is the meta object for an ItemSummary
type ItemSummaryMeta struct {
	CommentCount int64             `json:"totalComments"`
	ViewCount    int64             `json:"totalViews"`
	LastComment  interface{}       `json:"lastComment,omitempty"`
	Meta         h.SummaryMetaType `json:"meta"`
}

// LastComment encapsulates the last common on an item within a microcosm
type LastComment struct {
	ID int64 `json:"id"`
	h.CreatedType
	Valid bool `json:"-"`
}

// ItemDetail describes an item and the microcosm it belongs to
type ItemDetail struct {
	// Common Fields
	ID    int64  `json:"id"`
	Title string `json:"title"`

	ItemParent

	// Used during import to set the view count
	ViewCount int64 `json:"-"`
}

// ItemDetailCommentsAndMeta provides the comments for an item
type ItemDetailCommentsAndMeta struct {
	// Comments
	Comments h.ArrayType `json:"comments"`

	// Meta
	Meta h.DefaultMetaType `json:"meta"`
}

// Hydrate populates a partially populated Item
func (m *Item) Hydrate(siteID int64) (int, error) {
	profile, status, err := GetProfileSummary(siteID, m.Meta.CreatedByID)
	if err != nil {
		return status, err
	}
	m.Meta.CreatedBy = profile

	if m.LastCommentCreatedByNullable.Valid {
		profile, status, err :=
			GetProfileSummary(siteID, m.LastCommentCreatedByNullable.Int64)
		if err != nil {
			return status, err
		}
		m.LastCommentCreatedBy = profile
	}

	return http.StatusOK, nil
}

func (m *ItemParent) FetchBreadcrumb() (int, error) {
	if m.MicrocosmID > 0 {
		breadcrumb, status, err := getMicrocosmParents(m.MicrocosmID)
		if err != nil {
			return status, err
		}

		// Remove the top-level forums one
		if len(breadcrumb) > 0 {
			breadcrumb = breadcrumb[1:]

			m.Breadcrumb = &breadcrumb
		}
	}

	return http.StatusOK, nil
}

// IncrementViewCount increments the views of an item
func IncrementViewCount(itemTypeID int64, itemID int64) {
	// No transaction as we don't care for accuracy on these updates
	// Note: This function doesn't even return errors, we don't even care
	// if the occasional INSERT fails.
	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	// Integrity insert, gets rolled up and updated on cron
	_, err = db.Exec(
		`INSERT INTO views(item_type_id, item_id) VALUES ($1, $2)`,
		itemTypeID,
		itemID,
	)
	if err != nil {
		glog.Error(err)
		return
	}
}

// IncrementItemCommentCount increments the comments of an item
func IncrementItemCommentCount(itemTypeID int64, itemID int64) {

	if itemTypeID != h.ItemTypes[h.ItemTypeConversation] &&
		itemTypeID != h.ItemTypes[h.ItemTypeEvent] &&
		itemTypeID != h.ItemTypes[h.ItemTypePoll] &&
		itemTypeID != h.ItemTypes[h.ItemTypeQuestion] {
		return
	}

	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	switch itemTypeID {
	case h.ItemTypes[h.ItemTypeConversation]:
		_, err = db.Exec(`--Update Conversation Comment Count
UPDATE conversations
   SET comment_count = comment_count + 1
 WHERE conversation_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	case h.ItemTypes[h.ItemTypeEvent]:
		_, err = db.Exec(`--Update Event Comment Count
UPDATE events
   SET comment_count = comment_count + 1
 WHERE event_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	case h.ItemTypes[h.ItemTypePoll]:
		_, err = db.Exec(`--Update Poll Comment Count
UPDATE polls
   SET comment_count = comment_count + 1
 WHERE poll_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	case h.ItemTypes[h.ItemTypeQuestion]:
		_, err = db.Exec(`--Update Question Comment Count
UPDATE questions
   SET comment_count = comment_count + 1
 WHERE question_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	default:
		return
	}

	PurgeCache(itemTypeID, itemID)

	microcosmID := GetMicrocosmIDForItem(itemTypeID, itemID)
	if microcosmID > 0 {
		_, err = db.Exec(`--Update Microcosm Comment Count
UPDATE microcosms
   SET comment_count = comment_count + 1
 WHERE microcosm_id = $1`,
			microcosmID,
		)
		if err != nil {
			glog.Error(err)
			return
		}

		parents, _, err := getMicrocosmParents(microcosmID)
		if err != nil {
			glog.Error(err)
			return
		}
		for _, link := range parents {
			if link.Level > 1 {
				PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], link.ID)
			}
		}
	}
}

// DecrementItemCommentCount is called when a comment is deleted from an item
func DecrementItemCommentCount(itemTypeID int64, itemID int64) {

	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	switch itemTypeID {
	case h.ItemTypes[h.ItemTypeConversation]:
		_, err = db.Exec(`--Update Conversation Comment Count
UPDATE conversations
   SET comment_count = comment_count - 1
 WHERE conversation_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	case h.ItemTypes[h.ItemTypeEvent]:
		_, err = db.Exec(`--Update Event Comment Count
UPDATE events
   SET comment_count = comment_count - 1
 WHERE event_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	case h.ItemTypes[h.ItemTypePoll]:
		_, err = db.Exec(`--Update Poll Comment Count
UPDATE polls
   SET comment_count = comment_count - 1
 WHERE poll_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	case h.ItemTypes[h.ItemTypeQuestion]:
		_, err = db.Exec(`--Update Poll Comment Count
UPDATE questions
   SET comment_count = comment_count - 1
 WHERE question_id = $1`,
			itemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}
	default:
		glog.Error("Not yet implemented")
		return
	}

	PurgeCache(itemTypeID, itemID)

	microcosmID := GetMicrocosmIDForItem(itemTypeID, itemID)
	if microcosmID > 0 {
		_, err = db.Exec(`--Update Microcosm Comment Count
UPDATE microcosms
   SET comment_count = comment_count - 1
 WHERE microcosm_id = $1`,
			microcosmID,
		)
		if err != nil {
			glog.Error(err)
			return
		}

		parents, _, err := getMicrocosmParents(microcosmID)
		if err != nil {
			glog.Error(err)
			return
		}
		for _, link := range parents {
			if link.Level > 1 {
				PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], link.ID)
			}
		}
	}
}

// GetAllItems fetches items within a microcosm
func GetAllItems(
	siteID int64,
	microcosmID int64,
	profileID int64,
	limit int64,
	offset int64,
) (
	[]SummaryContainer,
	int64,
	int64,
	int,
	error,
) {
	// Retrieve resources
	db, err := h.GetConnection()
	if err != nil {
		return []SummaryContainer{}, 0, 0, http.StatusInternalServerError, err
	}

	sqlChildMicrocosms := `
WITH m AS (
    SELECT m.microcosm_id
          ,m.sequence
      FROM microcosms m
      LEFT JOIN permissions_cache p ON p.site_id = m.site_id
                                   AND p.item_type_id = 2
                                   AND p.item_id = m.microcosm_id
                                   AND p.profile_id = $3
           LEFT JOIN ignores_expanded i ON i.profile_id = $3
                                       AND i.item_type_id = 2
                                       AND i.item_id = m.microcosm_id
     WHERE m.site_id = $1
       AND m.parent_id = $2
       AND m.is_deleted IS NOT TRUE
       AND m.is_moderated IS NOT TRUE
       AND i.profile_id IS NULL
       AND (
               (p.can_read IS NOT NULL AND p.can_read IS TRUE)
            OR (get_effective_permissions($1,m.microcosm_id,2,m.microcosm_id,$3)).can_read IS TRUE
           )
),
p AS (
    SELECT $2::bigint AS microcosm_id
     WHERE (get_effective_permissions($1, $2, 2, $2, $3)).can_read IS TRUE
)`

	sqlFromWhere := `
          FROM flags f
          JOIN p ON f.microcosm_id = p.microcosm_id
          LEFT JOIN ignores i ON i.profile_id = $3
                             AND i.item_type_id = f.item_type_id
                             AND i.item_id = f.item_id
          LEFT JOIN m AS m2 ON f.item_type_id = 2
                           AND f.item_id = m2.microcosm_id
         WHERE f.microcosm_id = $2::BIGINT
           AND (
                   f.item_type_id IN (6, 9, 10)
                OR (
                       f.item_type_id = 2
                   AND m2.microcosm_id IS NOT NULL
                   )
               )
           AND f.site_id = $1
           AND i.profile_id IS NULL
           AND f.microcosm_is_deleted IS NOT TRUE
           AND f.microcosm_is_moderated IS NOT TRUE
           AND f.parent_is_deleted IS NOT TRUE
           AND f.parent_is_moderated IS NOT TRUE
           AND f.item_is_deleted IS NOT TRUE
           AND f.item_is_moderated IS NOT TRUE`

	var total int64
	err = db.QueryRow(sqlChildMicrocosms+`
SELECT COUNT(*) AS total`+sqlFromWhere,
		siteID,
		microcosmID,
		profileID,
	).Scan(
		&total,
	)
	if err != nil {
		glog.Error(err)
		return []SummaryContainer{}, 0, 0, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}

	rows, err := db.Query(sqlChildMicrocosms+`
SELECT item_type_id
      ,item_id
      ,has_unread(item_type_id, item_id, $3)
      ,CASE WHEN item_type_id = 9
           THEN is_attending(item_id, $3)
           ELSE FALSE
       END AS is_attending
  FROM (
        SELECT f.item_type_id
              ,f.item_id`+sqlFromWhere+`
         ORDER BY f.item_is_sticky DESC
                 ,m2.sequence ASC NULLS LAST
                 ,f.last_modified DESC
         LIMIT $4
        OFFSET $5
       ) r`,
		siteID,
		microcosmID,
		profileID,
		limit,
		offset,
	)
	if err != nil {
		glog.Error(err)
		return []SummaryContainer{}, 0, 0, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}
	defer rows.Close()

	var wg1 sync.WaitGroup
	req := make(chan SummaryContainerRequest)
	defer close(req)

	// [itemTypeId_itemId] = hasUnread
	unread := map[string]bool{}
	attending := map[int64]bool{}

	seq := 0
	for rows.Next() {
		var (
			itemTypeID  int64
			itemID      int64
			hasUnread   bool
			isAttending bool
		)

		err = rows.Scan(
			&itemTypeID,
			&itemID,
			&hasUnread,
			&isAttending,
		)
		if err != nil {
			return []SummaryContainer{}, 0, 0, http.StatusInternalServerError,
				fmt.Errorf("Row parsing error: %v", err.Error())
		}

		unread[strconv.FormatInt(itemTypeID, 10)+`_`+
			strconv.FormatInt(itemID, 10)] = hasUnread

		if itemTypeID == h.ItemTypes[h.ItemTypeEvent] {
			attending[itemID] = isAttending
		}

		go HandleSummaryContainerRequest(
			siteID,
			itemTypeID,
			itemID,
			profileID,
			seq,
			req,
		)
		seq++
		wg1.Add(1)
	}
	err = rows.Err()
	if err != nil {
		return []SummaryContainer{}, 0, 0, http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows: %v", err.Error())
	}
	rows.Close()

	resps := []SummaryContainerRequest{}
	for i := 0; i < seq; i++ {
		resp := <-req
		wg1.Done()
		resps = append(resps, resp)
	}
	wg1.Wait()

	for _, resp := range resps {
		if resp.Err != nil {
			return []SummaryContainer{}, 0, 0, resp.Status, resp.Err
		}
	}

	sort.Sort(SummaryContainerRequestsBySeq(resps))

	ems := []SummaryContainer{}
	for _, resp := range resps {
		m := resp.Item

		switch m.Summary.(type) {
		case MicrocosmSummaryType:
			summary := m.Summary.(MicrocosmSummaryType)
			summary.Meta.Flags.Unread =
				unread[strconv.FormatInt(m.ItemTypeID, 10)+`_`+
					strconv.FormatInt(m.ItemID, 10)]

			m.Summary = summary

		case ConversationSummaryType:
			summary := m.Summary.(ConversationSummaryType)
			summary.Meta.Flags.Unread =
				unread[strconv.FormatInt(m.ItemTypeID, 10)+`_`+
					strconv.FormatInt(m.ItemID, 10)]

			m.Summary = summary

		case EventSummaryType:
			summary := m.Summary.(EventSummaryType)
			summary.Meta.Flags.Attending = attending[m.ItemID]
			summary.Meta.Flags.Unread =
				unread[strconv.FormatInt(m.ItemTypeID, 10)+`_`+
					strconv.FormatInt(m.ItemID, 10)]

			m.Summary = summary

		case PollSummaryType:
			summary := m.Summary.(PollSummaryType)
			summary.Meta.Flags.Unread =
				unread[strconv.FormatInt(m.ItemTypeID, 10)+`_`+
					strconv.FormatInt(m.ItemID, 10)]

			m.Summary = summary

		default:
		}

		ems = append(ems, m)
	}

	pages := h.GetPageCount(total, limit)
	maxOffset := h.GetMaxOffset(total, limit)

	if offset > maxOffset {
		return []SummaryContainer{}, 0, 0, http.StatusBadRequest,
			fmt.Errorf("Not enough records, "+
				"offset (%d) would return an empty page",
				offset,
			)
	}

	return ems, total, pages, http.StatusOK, nil
}

// GetMostRecentItem fetches the most recently updated item within a microcosm
func GetMostRecentItem(
	siteID int64,
	microcosmID int64,
	profileID int64,
) (
	SummaryContainer,
	int,
	error,
) {
	// Retrieve resources
	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return SummaryContainer{}, http.StatusInternalServerError, err
	}

	// Fetch a summary of the most recent item that exists in this or a child
	// forum that this person may see.
	rows, err := db.Query(`--GetMostRecentItem
WITH m AS (
    SELECT m.microcosm_id
      FROM (
               SELECT path
                 FROM microcosms
                WHERE microcosm_id = $2
           ) im
      JOIN microcosms m ON m.path <@ im.path
      LEFT JOIN permissions_cache p ON p.site_id = m.site_id
                                   AND p.item_type_id = 2
                                   AND p.item_id = m.microcosm_id
                                   AND p.profile_id = $3
      LEFT JOIN ignores_expanded i ON i.profile_id = $3
                                  AND i.item_type_id = 2
                                  AND i.item_id = m.microcosm_id
     WHERE m.site_id = $1
       AND m.is_deleted IS NOT TRUE
       AND m.is_moderated IS NOT TRUE
       AND i.profile_id IS NULL
       AND (
               (p.can_read IS NOT NULL AND p.can_read IS TRUE)
            OR (get_effective_permissions($1,m.microcosm_id,2,m.microcosm_id,$3)).can_read IS TRUE
           )
)
SELECT item_type_id
      ,item_id
  FROM flags
 WHERE microcosm_id IN (SELECT microcosm_id FROM m)
   AND item_is_deleted IS NOT TRUE
   AND item_is_moderated IS NOT TRUE
   AND (item_type_id = 6 OR item_type_id = 9)
 ORDER BY last_modified DESC
 LIMIT 1`,
		siteID,
		microcosmID,
		profileID,
	)
	if err != nil {
		glog.Errorf("db.Query(%d) %+v", microcosmID, err)
		return SummaryContainer{},
			http.StatusInternalServerError,
			fmt.Errorf("Database query failed")
	}
	defer rows.Close()

	var (
		m          SummaryContainer
		status     int
		itemTypeID int64
		itemID     int64
	)
	for rows.Next() {
		err = rows.Scan(
			&itemTypeID,
			&itemID,
		)
		if err != nil {
			glog.Errorf("rows.Scan() %+v", err)
			return SummaryContainer{},
				http.StatusInternalServerError,
				fmt.Errorf("Row parsing error")
		}

		m, status, err =
			GetSummaryContainer(siteID, itemTypeID, itemID, profileID)
		if err != nil {
			glog.Errorf(
				"GetSummaryContainer(%d, %d, %d, %d) %+v",
				siteID,
				itemTypeID,
				itemID,
				profileID,
				err,
			)
			return SummaryContainer{}, status, err
		}
	}
	err = rows.Err()
	if err != nil {
		glog.Errorf("rows.Err() %+v", err)
		return SummaryContainer{},
			http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows")
	}
	rows.Close()

	if itemID == 0 {
		glog.Info("itemId == 0")
		return SummaryContainer{}, http.StatusNotFound, nil
	}

	return m, http.StatusOK, nil
}

// GetItems fetches items for a microcosm
func GetItems(
	siteID int64,
	microcosmID int64,
	profileID int64,
	reqURL *url.URL,
) (
	h.ArrayType,
	int,
	error,
) {
	query := reqURL.Query()
	limit, offset, status, err := h.GetLimitAndOffset(query)
	if err != nil {
		return h.ArrayType{}, status, err
	}

	ems, total, pages, status, err :=
		GetAllItems(siteID, microcosmID, profileID, limit, offset)
	if err != nil {
		return h.ArrayType{}, status, err
	}

	m := h.ConstructArray(
		ems,
		h.APITypeComment,
		total,
		limit,
		offset,
		pages,
		reqURL,
	)

	return m, http.StatusOK, nil
}
